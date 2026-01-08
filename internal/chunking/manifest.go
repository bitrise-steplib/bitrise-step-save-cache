package chunking

import (
	"encoding/json"
)

// Manifest represents the metadata for a chunked cache archive
type Manifest struct {
	Version   int     `json:"version"`
	Algorithm string  `json:"algorithm"`
	TotalSize int64   `json:"total_size"`
	Chunks    []Chunk `json:"chunks"`
}

// NewManifest creates a new manifest with the given chunks
func NewManifest(chunks []Chunk, totalSize int64) *Manifest {
	return &Manifest{
		Version:   1,
		Algorithm: "fastcdc",
		TotalSize: totalSize,
		Chunks:    chunks,
	}
}

// ToJSON serializes the manifest to JSON
func (m *Manifest) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// FromJSON deserializes a manifest from JSON
func FromJSON(data []byte) (*Manifest, error) {
	var manifest Manifest
	err := json.Unmarshal(data, &manifest)
	if err != nil {
		return nil, err
	}
	return &manifest, nil
}

// GetChunkHashes returns all chunk hashes from the manifest
func (m *Manifest) GetChunkHashes() []string {
	hashes := make([]string, len(m.Chunks))
	for i, chunk := range m.Chunks {
		hashes[i] = chunk.Hash
	}
	return hashes
}
