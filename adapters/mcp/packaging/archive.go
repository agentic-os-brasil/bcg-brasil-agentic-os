package packaging

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"sort"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// ValidateArchive reads an emitted MCPB without extracting, repairing or
// executing it. Both filesystem and ZIP inputs use the same closed inventory
// and canonical-payload validator; neither confers publisher or owner authority.
func ValidateArchive(path string) (PacketStatus, error) {
	file, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return PacketStatus{}, err
	}
	defer file.Close()
	if err := scheduler.ValidateSingleLink(file); err != nil {
		return PacketStatus{}, err
	}
	before, err := file.Stat()
	if err != nil {
		return PacketStatus{}, err
	}
	if before.Size() <= 0 || before.Size() > maximumPacketBytes {
		return PacketStatus{}, errors.New("packet archive bound")
	}
	archive, err := zip.NewReader(file, before.Size())
	if err != nil {
		return PacketStatus{}, err
	}
	entries := map[string]*zip.File{}
	var total uint64
	for _, entry := range archive.File {
		if !packetPath(entry.Name) || !entry.Mode().IsRegular() || entry.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || (entry.Mode().Perm() != 0644 && entry.Mode().Perm() != 0755) || entry.UncompressedSize64 > uint64(maximumPacketBytes) {
			return PacketStatus{}, errors.New("packet archive entry role or bound")
		}
		if _, exists := entries[entry.Name]; exists {
			return PacketStatus{}, errors.New("duplicate packet archive entry")
		}
		total += entry.UncompressedSize64
		if total > uint64(maximumPacketBytes) || len(entries) >= maximumPacketFiles+1 {
			return PacketStatus{}, errors.New("packet archive bounds")
		}
		entries[entry.Name] = entry
	}
	read := func(name string, limit int64) ([]byte, error) {
		entry := entries[name]
		if entry == nil {
			return nil, errors.New("packet archive member unavailable")
		}
		if entry.UncompressedSize64 > uint64(limit) {
			return nil, errors.New("packet archive member bound")
		}
		reader, err := entry.Open()
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(reader, limit+1))
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			return nil, errors.Join(readErr, closeErr)
		}
		if int64(len(body)) > limit || uint64(len(body)) != entry.UncompressedSize64 {
			return nil, errors.New("packet archive member changed")
		}
		return body, nil
	}
	status, err := validatePacket(packetSource{
		read: read, exactModes: true,
		scan: func() ([]packetFile, error) {
			var files []packetFile
			for name, entry := range entries {
				if name == InventoryName {
					continue
				}
				body, err := read(name, maximumPacketBytes)
				if err != nil {
					return nil, err
				}
				files = append(files, packetFile{name, digest(body), int64(len(body)), uint32(entry.Mode().Perm())})
			}
			sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
			return files, nil
		},
	})
	if err != nil {
		return PacketStatus{}, err
	}
	after, err := file.Stat()
	if err != nil || !sameHostMetadata(before, after) {
		return PacketStatus{}, errors.New("packet archive changed while reading")
	}
	return status, nil
}
