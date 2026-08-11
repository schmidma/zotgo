package output

import (
	"strings"

	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

// CollectionPath is one requested collection's stable root-to-leaf path.
type CollectionPath struct {
	Key         string                  `json:"key"`
	Name        string                  `json:"name"`
	ParentKey   string                  `json:"parentKey"`
	Segments    []CollectionPathSegment `json:"segments"`
	DisplayPath string                  `json:"displayPath"`
}

// CollectionPathSegment identifies one collection in a path.
type CollectionPathSegment struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// NewCollectionPaths converts resolved collection paths to stable DTOs.
func NewCollectionPaths(paths []zotero.CollectionPath) []CollectionPath {
	records := make([]CollectionPath, 0, len(paths))
	for _, path := range paths {
		segments := make([]CollectionPathSegment, 0, len(path.Segments))
		names := make([]string, 0, len(path.Segments))
		for _, segment := range path.Segments {
			segments = append(segments, CollectionPathSegment{Key: segment.Key, Name: segment.Name})
			names = append(names, segment.Name)
		}
		records = append(records, CollectionPath{
			Key:         path.Key,
			Name:        path.Name,
			ParentKey:   path.ParentKey,
			Segments:    segments,
			DisplayPath: strings.Join(names, " / "),
		})
	}
	return records
}
