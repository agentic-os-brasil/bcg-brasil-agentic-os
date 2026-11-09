package product

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maximumContentArtifact = 140 << 20

func contentQueries() []any {
	return []any{
		variant("content.status", map[string]any{}),
		variant("content.read", map[string]any{"artifact_id": strSchema(64), "format": map[string]any{"type": "string", "enum": []string{"markdown", "document"}}, "offset": map[string]any{"type": "integer", "minimum": 0, "maximum": maximumContentArtifact}, "max_bytes": map[string]any{"type": "integer", "minimum": 4, "maximum": 16 << 10}}, "artifact_id", "format"),
		variant("knowledge.collect", map[string]any{"purpose": strSchema(768), "pages": map[string]any{"type": "array", "minItems": 1, "maxItems": 24, "uniqueItems": true, "items": strSchema(256)}}, "purpose", "pages"),
	}
}
func knowledgeMutations() []any {
	return []any{
		variant("knowledge.create", map[string]any{"artifact_id": strSchema(64), "page": strSchema(256), "text": strSchema(256 << 10)}, "artifact_id", "page", "text"),
		variant("knowledge.append", map[string]any{"artifact_id": strSchema(64), "page": strSchema(256), "section": strSchema(256), "text": strSchema(8 << 10), "expected_page_revision": strSchema(64)}, "artifact_id", "page", "section", "text", "expected_page_revision"),
	}
}
func contentMutations() []any {
	return append([]any{variant("content.convert", map[string]any{"source_path": strSchema(256)}, "source_path")}, knowledgeMutations()...)
}

// KnowledgeMutation carries domain input only. Host approval and provenance are server-owned.
type KnowledgeMutation struct {
	Operation            string `json:"operation"`
	ArtifactID           string `json:"artifact_id"`
	Page                 string `json:"page"`
	Section              string `json:"section,omitempty"`
	Text                 string `json:"text"`
	ExpectedPageRevision string `json:"expected_page_revision,omitempty"`
}

func DecodeKnowledgeMutation(body []byte) (KnowledgeMutation, error) {
	var in KnowledgeMutation
	if len(body) > 2<<20 || !utf8.Valid(body) {
		return in, errors.New("knowledge_input_size")
	}
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return in, err
	}
	schemaBody, _ := json.Marshal(map[string]any{"type": "object", "oneOf": knowledgeMutations()})
	var definition jsonschema.Schema
	if err := json.Unmarshal(schemaBody, &definition); err != nil {
		return in, err
	}
	resolved, err := definition.Resolve(nil)
	if err != nil {
		return in, err
	}
	if err = resolved.Validate(raw); err != nil {
		return in, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&in); err != nil {
		return in, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return in, errors.New("knowledge_trailing_input")
	}
	return in, nil
}

type contentArtifact struct {
	Scope         string             `json:"scope"`
	WorkspaceID   string             `json:"workspace_id"`
	WorkspacePath string             `json:"workspace_path"`
	SourcePath    string             `json:"source_path"`
	Result        ingest.Result      `json:"result"`
	Provenance    docling.Provenance `json:"provenance"`
	Markdown      []byte             `json:"markdown"`
	Document      []byte             `json:"document"`
}

func contentDigest(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }
func validContentDigest(s string) bool {
	v, err := hex.DecodeString(s)
	return err == nil && len(v) == 32 && s == strings.ToLower(s)
}
func (b *Bound) contentDirectory() string {
	return filepath.Join(b.home, "state", "content", b.workspaceID, "artifacts")
}
func (b *Bound) publishContent(ctx context.Context, source string, conversion docling.Conversion) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := b.validate(); err != nil {
		return "", err
	}
	artifact := contentArtifact{Scope: b.home, WorkspaceID: b.workspaceID, WorkspacePath: b.options.WorkspacePath, SourcePath: source, Result: conversion.Result, Provenance: conversion.Provenance, Markdown: conversion.Markdown, Document: conversion.Document}
	if err := b.validateContent(artifact); err != nil {
		return "", err
	}
	if err := b.checkContentSource(ctx, artifact); err != nil {
		return "", err
	}
	body, err := json.Marshal(artifact)
	if err != nil {
		return "", err
	}
	if len(body) > maximumContentArtifact {
		return "", errors.New("artifact_budget")
	}
	id := contentDigest(body)
	directory := b.contentDirectory()
	if err := scheduler.EnsurePrivateDirectory(directory); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := scheduler.PublishNewPrivateFile(filepath.Join(directory, id+".json"), body); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
		if _, err = b.readContent(id); err != nil {
			return "", err
		}
	}
	return id, nil
}
func (b *Bound) validateContent(a contentArtifact) error {
	if err := b.workspacePath(a.SourcePath); err != nil {
		return err
	}
	if a.Scope != b.home || a.WorkspaceID != b.workspaceID || a.WorkspacePath != b.options.WorkspacePath || artifactPath(a.SourcePath) != nil || a.Result.WorkspaceID != b.workspaceID || a.Result.Status != ingest.StatusUsable || a.Result.Adapter != "docling" || !a.Provenance.NetworkDenied || !validContentDigest(a.Result.SourceSHA256) || !validContentDigest(a.Provenance.PackSHA256) || !validContentDigest(a.Provenance.ManifestSHA256) || !validContentDigest(a.Provenance.LockSHA256) || a.Provenance.MarkdownSHA256 != contentDigest(a.Markdown) || a.Provenance.DocumentSHA256 != contentDigest(a.Document) || !utf8.Valid(a.Markdown) || !utf8.Valid(a.Document) || !json.Valid(a.Document) || len(a.Markdown) == 0 || int64(len(a.Markdown)+len(a.Document)) > ingest.DefaultPolicy().MaxOutputBytes || a.Result.OutputBytes != int64(len(a.Markdown)+len(a.Document)) {
		return errors.New("artifact_unavailable")
	}
	return nil
}
func (b *Bound) readContent(id string) (contentArtifact, error) {
	var a contentArtifact
	if err := b.validate(); err != nil {
		return a, err
	}
	if !validContentDigest(id) {
		return a, errors.New("artifact_unavailable")
	}
	if err := scheduler.ValidatePrivateDirectory(b.contentDirectory()); err != nil {
		return a, err
	}
	body, err := scheduler.ReadPrivateFile(filepath.Join(b.contentDirectory(), id+".json"), maximumContentArtifact)
	if err != nil {
		return a, err
	}
	if contentDigest(body) != id {
		return a, errors.New("artifact_changed")
	}
	if err = json.Unmarshal(body, &a); err != nil {
		return a, err
	}
	return a, b.validateContent(a)
}
func (b *Bound) checkContentSource(ctx context.Context, a contentArtifact) error {
	return b.checkSelectedContentSource(ctx, a.SourcePath, a.Result.SourceSHA256)
}
func (b *Bound) checkSelectedContentSource(ctx context.Context, source, digest string) error {
	if err := b.validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := b.workspacePath(source); err != nil {
		return err
	}
	if err := b.DoclingRuntime().RevalidateSource(ctx, ingest.Request{SourcePath: source, WorkspacePath: b.options.WorkspacePath}, b.workspaceID, digest); err != nil {
		return err
	}
	if err := b.workspacePath(source); err != nil {
		return err
	}
	return b.validate()
}
func (b *Bound) knowledgeEngine() (*atlasops.Engine, error) { return b.workspaceEngine(nil) }

type KnowledgeReview struct {
	Scope        string                   `json:"scope"`
	WorkspaceID  string                   `json:"workspace_id"`
	Mutation     KnowledgeMutation        `json:"mutation"`
	SourcePath   string                   `json:"source_path"`
	SourceSHA256 string                   `json:"source_sha256"`
	Conversion   docling.Provenance       `json:"conversion"`
	Current      *atlasops.PageProjection `json:"current,omitempty"`
	Provenance   atlasops.Provenance      `json:"provenance"`
	ReviewDigest string                   `json:"review_digest"`
}

func (b *Bound) KnowledgeReview(in KnowledgeMutation) (KnowledgeReview, error) {
	return b.knowledgeReview(context.Background(), in)
}
func (b *Bound) knowledgeReview(ctx context.Context, in KnowledgeMutation) (KnowledgeReview, error) {
	var r KnowledgeReview
	encoded, err := json.Marshal(in)
	if err != nil {
		return r, err
	}
	if _, err = DecodeKnowledgeMutation(encoded); err != nil {
		return r, err
	}
	if artifactPath(in.Page) != nil || !strings.HasSuffix(in.Page, ".md") || !utf8.ValidString(in.Text) || strings.ContainsRune(in.Text, 0) || strings.TrimSpace(in.Text) == "" || len(in.Text) > 256<<10 || in.Operation == "knowledge.append" && (len(in.Text) > 8<<10 || !validContentDigest(in.ExpectedPageRevision) || !strings.HasPrefix(in.Section, "#")) {
		return r, errors.New("knowledge_invalid_input")
	}
	if err := b.workspacePage(in.Page); err != nil {
		return r, err
	}
	a, err := b.readContent(in.ArtifactID)
	if err != nil {
		return r, err
	}
	if err = b.checkContentSource(ctx, a); err != nil {
		return r, err
	}
	r = KnowledgeReview{Scope: b.home, WorkspaceID: b.workspaceID, Mutation: in, SourcePath: a.SourcePath, SourceSHA256: a.Result.SourceSHA256, Conversion: a.Provenance}
	// Intent identity excludes the transport and current page: retries reach the
	// canonical journal with exactly the original request and provenance.
	intent, _ := json.Marshal(r)
	id := contentDigest(intent)
	r.Provenance = atlasops.Provenance{Origin: atlasops.OriginAttended, SessionID: "content-" + id, IdempotencyKey: id, Ritual: "content:" + in.ArtifactID}
	engine, err := b.knowledgeEngine()
	if err != nil {
		return r, err
	}
	current, err := engine.ReviewPage(atlasops.CollectRequest{Purpose: "Review exact knowledge change", Reader: atlasops.ReaderOwnerSession, Pages: []string{in.Page}})
	if err == nil {
		r.Current = &current
	} else if !errors.Is(err, os.ErrNotExist) {
		return r, err
	}
	if in.Operation == "knowledge.append" && r.Current == nil {
		return r, errors.New("page_unavailable")
	}
	body, _ := json.Marshal(r)
	r.ReviewDigest = contentDigest(body)
	return r, nil
}
func (b *Bound) ApplyKnowledgeReview(in KnowledgeMutation, digest string, confirmed bool) (atlasops.Result, error) {
	return b.applyKnowledgeReview(context.Background(), in, digest, confirmed)
}
func (b *Bound) applyKnowledgeReview(ctx context.Context, in KnowledgeMutation, digest string, confirmed bool) (atlasops.Result, error) {
	if !confirmed {
		return atlasops.Result{}, errors.New("confirmation_required")
	}
	r, err := b.knowledgeReview(ctx, in)
	if err != nil {
		return atlasops.Result{}, err
	}
	if digest == "" || digest != r.ReviewDigest {
		return atlasops.Result{}, errOwnerReviewChanged
	}
	engine, err := b.workspaceEngine(func() error { return b.checkSelectedContentSource(ctx, r.SourcePath, r.SourceSHA256) })
	if err != nil {
		return atlasops.Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return atlasops.Result{}, err
	}
	if in.Operation == "knowledge.create" {
		return engine.CreatePage(atlasops.CreatePageRequest{Page: in.Page, Body: in.Text, Provenance: r.Provenance})
	}
	return engine.AppendEntry(atlasops.AppendEntryRequest{Page: in.Page, Section: in.Section, Entry: in.Text, ExpectedRevision: in.ExpectedPageRevision, Provenance: r.Provenance})
}
func knowledgePresentation(r KnowledgeReview) map[string]any {
	current := "Page does not exist"
	if r.Current != nil {
		current = r.Current.Content
	}
	return map[string]any{"Operation": r.Mutation.Operation, "Workspace": r.WorkspaceID, "Source document": r.SourcePath, "Source SHA-256": r.SourceSHA256, "Converted artifact": r.Mutation.ArtifactID, "Runtime pack SHA-256": r.Conversion.PackSHA256, "Page": r.Mutation.Page, "Section": r.Mutation.Section, "Current page": current, "Exact proposed text": r.Mutation.Text, "Expected page revision": r.Mutation.ExpectedPageRevision}
}
func contentProgress(ctx context.Context, req *mcp.CallToolRequest, progress float64, message string) {
	if req.Session != nil && req.Params.GetProgressToken() != nil {
		_ = req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{ProgressToken: req.Params.GetProgressToken(), Progress: progress, Total: 2, Message: message})
	}
}
func (b *Bound) handleContent(ctx context.Context, req *mcp.CallToolRequest, in input) *mcp.CallToolResult {
	var value any
	var err error
	if err = ctx.Err(); err == nil {
		switch in.Operation {
		case "content.status":
			value = b.contentStatus(ctx)
		case "content.convert":
			if err = b.workspacePath(in.SourcePath); err != nil {
				break
			}
			contentProgress(ctx, req, 0, "Verifying the local runtime and converting the selected document")
			var conversion docling.Conversion
			conversion, err = b.DoclingRuntime().Convert(ctx, ingest.Request{SourcePath: in.SourcePath, WorkspacePath: b.options.WorkspacePath, Policy: ingest.DefaultPolicy()}, b.workspaceID)
			if err == nil {
				err = ctx.Err()
			}
			if err == nil && conversion.Result.Status == ingest.StatusUsable {
				contentProgress(ctx, req, 1, "Publishing the verified workspace artifact")
				var id string
				id, err = b.publishContent(ctx, in.SourcePath, conversion)
				conversion.Result.ArtifactRef = id
			}
			value = map[string]any{"result": conversion.Result, "provenance": conversion.Provenance}
			if err == nil {
				contentProgress(ctx, req, 2, "Conversion finished")
			}
		case "content.read":
			var a contentArtifact
			a, err = b.readContent(in.ArtifactID)
			if err == nil {
				body := a.Markdown
				if in.Format == "document" {
					body = a.Document
				}
				size := in.MaxBytes
				if size == 0 {
					size = 16 << 10
				}
				if in.Offset < 0 || in.Offset > len(body) || in.Offset < len(body) && !utf8.RuneStart(body[in.Offset]) {
					err = errors.New("artifact_offset")
					break
				}
				end := min(in.Offset+size, len(body))
				for end < len(body) && !utf8.RuneStart(body[end]) {
					end--
				}
				value = map[string]any{"artifact_id": in.ArtifactID, "format": in.Format, "text": string(body[in.Offset:end]), "next_offset": end, "has_more": end < len(body), "result": a.Result, "provenance": a.Provenance}
			}
		case "knowledge.collect":
			var engine *atlasops.Engine
			engine, err = b.knowledgeEngine()
			if err == nil {
				value, err = engine.Collect(atlasops.CollectRequest{Purpose: in.Purpose, Reader: atlasops.ReaderOwnerSession, Pages: in.Pages})
			}
		case "knowledge.create", "knowledge.append":
			mutation := KnowledgeMutation{Operation: in.Operation, ArtifactID: in.ArtifactID, Page: in.Page, Section: in.Section, Text: in.Text, ExpectedPageRevision: in.ExpectedPageRevision}
			var review KnowledgeReview
			review, err = b.knowledgeReview(ctx, mutation)
			if err == nil {
				err = b.confirm(ctx, req, "Authorize this exact workspace knowledge change?", review, knowledgePresentation(review))
			}
			if err == nil {
				var applied atlasops.Result
				applied, err = b.applyKnowledgeReview(ctx, mutation, review.ReviewDigest, true)
				// Full-page conflict proposals belong only to private operator review.
				applied.Proposal = ""
				value = applied
			}
		default:
			err = errors.New("invalid_operation")
		}
	}
	if err == nil {
		return result(value, nil)
	}
	var pending *confirmationPending
	if errors.As(err, &pending) {
		return pending.result
	}
	code := "content_rejected"
	switch {
	case errors.Is(err, errOwnerReviewChanged):
		code = "revision_conflict"
	case errors.Is(err, errConfirmationUnavailable):
		code = "confirmation_unavailable"
	case errors.Is(err, errConfirmationDeclined):
		code = "confirmation_declined"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		code = "cancelled"
	}
	metadata := map[string]string{"code": code}
	if code == "confirmation_unavailable" {
		metadata["next_action"] = "Use knowledge-review then knowledge-apply with the same JSON on stdin, exact --review-digest and explicit --confirm."
	}
	out := result(metadata, nil)
	out.IsError = true
	return out
}
