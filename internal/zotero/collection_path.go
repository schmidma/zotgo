package zotero

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// CollectionPathSegment is one collection in a root-to-leaf path.
type CollectionPathSegment struct {
	Key  string
	Name string
}

// CollectionPath is one requested collection and its root-to-leaf ancestry.
type CollectionPath struct {
	Key       string
	Name      string
	ParentKey string
	Segments  []CollectionPathSegment
}

// ResolveRawCollectionPaths projects only key and data from raw collection
// envelopes, avoiding unrelated endpoint-scoped version decoding.
func ResolveRawCollectionPaths(rawEnvelopes []json.RawMessage, keys []string) ([]CollectionPath, error) {
	envelopes := make([]Envelope, 0, len(rawEnvelopes))
	for i, raw := range rawEnvelopes {
		var envelope struct {
			Key  string          `json:"key"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, fmt.Errorf("decode collection envelope %d: %w", i, err)
		}
		envelopes = append(envelopes, Envelope{Key: envelope.Key, Data: envelope.Data})
	}
	return ResolveCollectionPaths(envelopes, keys)
}

// ResolveCollectionPaths resolves requested collection keys from a complete
// collection listing. Results preserve request order.
func ResolveCollectionPaths(envelopes []Envelope, keys []string) ([]CollectionPath, error) {
	collections := make(map[string]Envelope, len(envelopes))
	for _, envelope := range envelopes {
		if envelope.Key == "" {
			return nil, fmt.Errorf("collection response has no key")
		}
		if _, exists := collections[envelope.Key]; exists {
			return nil, fmt.Errorf("collection response contains duplicate key %q", envelope.Key)
		}
		collections[envelope.Key] = envelope
	}

	paths := make([]CollectionPath, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			return nil, fmt.Errorf("collection key must not be empty")
		}
		path, err := resolveCollectionPath(collections, key)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func resolveCollectionPath(collections map[string]Envelope, requestedKey string) (CollectionPath, error) {
	visited := make(map[string]struct{})
	reversed := make([]CollectionPathSegment, 0)
	currentKey := requestedKey
	leafParentKey := ""
	leafName := ""
	for currentKey != "" {
		if _, seen := visited[currentKey]; seen {
			return CollectionPath{}, fmt.Errorf("collection path for %q contains a cycle at %q", requestedKey, currentKey)
		}
		visited[currentKey] = struct{}{}

		envelope, ok := collections[currentKey]
		if !ok {
			if currentKey == requestedKey {
				return CollectionPath{}, fmt.Errorf("no collection with key %q", requestedKey)
			}
			return CollectionPath{}, fmt.Errorf("collection path for %q references missing parent %q", requestedKey, currentKey)
		}
		trimmedData := bytes.TrimSpace(envelope.Data)
		if len(trimmedData) == 0 || bytes.Equal(trimmedData, []byte("null")) || trimmedData[0] != '{' {
			return CollectionPath{}, fmt.Errorf("decode collection %q: expected a data object", currentKey)
		}
		data, err := envelope.CollectionData()
		if err != nil {
			return CollectionPath{}, fmt.Errorf("decode collection %q: %w", currentKey, err)
		}
		if data.Key == "" {
			return CollectionPath{}, fmt.Errorf("collection %q data has no key", currentKey)
		}
		if data.Key != currentKey {
			return CollectionPath{}, fmt.Errorf("collection %q data has key %q", currentKey, data.Key)
		}
		if data.Name == "" {
			return CollectionPath{}, fmt.Errorf("collection %q data has no name", currentKey)
		}
		parentKey, err := strictCollectionParentKey(data.ParentCollection)
		if err != nil {
			return CollectionPath{}, fmt.Errorf("collection %q parentCollection: %w", currentKey, err)
		}
		if currentKey == requestedKey {
			leafName = data.Name
			leafParentKey = parentKey
		}
		reversed = append(reversed, CollectionPathSegment{Key: currentKey, Name: data.Name})
		currentKey = parentKey
	}

	segments := make([]CollectionPathSegment, len(reversed))
	for i := range reversed {
		segments[len(reversed)-1-i] = reversed[i]
	}
	return CollectionPath{
		Key:       requestedKey,
		Name:      leafName,
		ParentKey: leafParentKey,
		Segments:  segments,
	}, nil
}

func strictCollectionParentKey(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if bytes.Equal(raw, []byte("false")) {
		return "", nil
	}
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", fmt.Errorf("expected a non-empty collection key or false")
	}
	var key string
	if err := json.Unmarshal(raw, &key); err != nil || key == "" {
		return "", fmt.Errorf("expected a non-empty collection key or false")
	}
	return key, nil
}
