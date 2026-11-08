package product_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type contentConversionProgress struct {
	mu       sync.Mutex
	values   []float64
	terminal chan struct{}
}

func (p *contentConversionProgress) record(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.values = append(p.values, req.Params.Progress)
	if req.Params.Progress == 2 {
		select {
		case p.terminal <- struct{}{}:
		default:
		}
	}
}

func (p *contentConversionProgress) wait(ctx context.Context) ([]float64, error) {
	var err error
	select {
	case <-p.terminal:
	case <-ctx.Done():
		err = ctx.Err()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]float64(nil), p.values...), err
}

// Signal entry into the waiter without relying on scheduler timing or sleeps.
type contentProgressWaitContext struct {
	context.Context
	waiting chan struct{}
}

func (c contentProgressWaitContext) Done() <-chan struct{} {
	select {
	case c.waiting <- struct{}{}:
	default:
	}
	return c.Context.Done()
}

func TestContentProgressWaitsForHandlerAfterToolResponse(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	terminalEntered := make(chan struct{})
	releaseTerminal := make(chan struct{})
	release := sync.OnceFunc(func() { close(releaseTerminal) })
	defer release()
	progress := contentConversionProgress{terminal: make(chan struct{}, 1)}
	client := mcp.NewClient(&mcp.Implementation{Name: "content-progress-test", Version: "1"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(ctx context.Context, req *mcp.ProgressNotificationClientRequest) {
			if req.Params.Progress == 2 {
				close(terminalEntered)
				select {
				case <-releaseTerminal:
				case <-ctx.Done():
					return
				}
			}
			progress.record(ctx, req)
		},
	})
	server := mcp.NewServer(&mcp.Implementation{Name: "content-progress-test", Version: "1"}, nil)
	server.AddTool(&mcp.Tool{Name: "convert", InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		for _, value := range []float64{0, 1, 2} {
			if err := req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{ProgressToken: req.Params.GetProgressToken(), Progress: value, Total: 2}); err != nil {
				return nil, err
			}
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "usable"}}}, nil
	})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	params := &mcp.CallToolParams{Name: "convert", Arguments: map[string]any{}}
	params.SetProgressToken("conversion")
	result, err := cs.CallTool(ctx, params)
	if err != nil || result.IsError {
		t.Fatalf("tool response: %+v %v", result, err)
	}
	select {
	case <-terminalEntered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	progress.mu.Lock()
	before := append([]float64(nil), progress.values...)
	progress.mu.Unlock()
	if !reflect.DeepEqual(before, []float64{0, 1}) {
		t.Fatalf("terminal callback was not controlled: %v", before)
	}
	waiting := make(chan struct{}, 1)
	go func() {
		select {
		case <-waiting:
		case <-ctx.Done():
		}
		release()
	}()
	got, err := progress.wait(contentProgressWaitContext{Context: ctx, waiting: waiting})
	if err != nil || !reflect.DeepEqual(got, []float64{0, 1, 2}) {
		t.Fatalf("progress was observed before its queued handler completed: %v %v", got, err)
	}
}

func TestContentProgressMissingTerminalHonorsContext(t *testing.T) {
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	expired, stop := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer stop()
	for _, test := range []struct {
		name string
		ctx  context.Context
		want error
	}{{"canceled", canceled, context.Canceled}, {"deadline", expired, context.DeadlineExceeded}} {
		t.Run(test.name, func(t *testing.T) {
			progress := contentConversionProgress{values: []float64{0, 1}, terminal: make(chan struct{}, 1)}
			got, err := progress.wait(test.ctx)
			if !errors.Is(err, test.want) || !reflect.DeepEqual(got, []float64{0, 1}) {
				t.Fatalf("missing terminal was accepted or lost context: %v %v", got, err)
			}
		})
	}
}
