package zotero

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// versionNumber accepts the empty version emitted for untouched objects
// migrated by affected Zotero 10 beta builds, while rejecting other strings.
type versionNumber int

func (v *versionNumber) UnmarshalJSON(data []byte) error {
	if string(data) == `""` {
		*v = 0
		return nil
	}
	var number int
	if err := json.Unmarshal(data, &number); err != nil {
		return err
	}
	*v = versionNumber(number)
	return nil
}

// Envelope is the common Local API wrapper for items and collections.
//
// Data is intentionally raw: Zotero item fields vary by itemType, and preserving
// unknown fields is safer than flattening them away.
type Envelope struct {
	Key     string                     `json:"key"`
	Version int                        `json:"version"`
	Library Library                    `json:"library"`
	Links   map[string]Link            `json:"links"`
	Meta    map[string]json.RawMessage `json:"meta"`
	Data    json.RawMessage            `json:"data"`
}

func (e *Envelope) UnmarshalJSON(data []byte) error {
	type envelope Envelope
	decoded := struct {
		Version versionNumber `json:"version"`
		*envelope
	}{envelope: (*envelope)(e)}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	e.Version = int(decoded.Version)
	return nil
}

// Library identifies the library that owns an envelope.
type Library struct {
	Type  string          `json:"type"`
	ID    int64           `json:"id"`
	Name  string          `json:"name"`
	Links map[string]Link `json:"links"`
}

// Link is a Zotero Local API link object. Attachment links include extra fields.
type Link struct {
	Href           string `json:"href"`
	Type           string `json:"type"`
	Title          string `json:"title"`
	Length         *int64 `json:"length"`
	AttachmentType string `json:"attachmentType"`
	AttachmentSize int64  `json:"attachmentSize"`
}

// ItemData contains the stable item fields zotgo needs for display and tests.
type ItemData struct {
	Key         string    `json:"key"`
	Version     int       `json:"version"`
	ItemType    string    `json:"itemType"`
	Title       string    `json:"title"`
	Date        string    `json:"date"`
	Creators    []Creator `json:"creators"`
	Tags        []Tag     `json:"tags"`
	Collections []string  `json:"collections"`
}

func (d *ItemData) UnmarshalJSON(data []byte) error {
	type itemData ItemData
	decoded := struct {
		Version versionNumber `json:"version"`
		*itemData
	}{itemData: (*itemData)(d)}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	d.Version = int(decoded.Version)
	return nil
}

// Relation is one predicate/target edge from an item.
type Relation struct {
	Predicate string
	Target    string
	TargetKey string
}

// Annotation is the compact annotation metadata used by list output.
type Annotation struct {
	Key           string
	AttachmentKey string
	Type          string
	PageLabel     string
	Color         string
	SortIndex     string
	HasText       bool
	HasComment    bool
}

type annotationData struct {
	ItemType            string `json:"itemType"`
	ParentItem          string `json:"parentItem"`
	AnnotationType      string `json:"annotationType"`
	AnnotationText      string `json:"annotationText"`
	AnnotationComment   string `json:"annotationComment"`
	AnnotationColor     string `json:"annotationColor"`
	AnnotationPageLabel string `json:"annotationPageLabel"`
	AnnotationSortIndex string `json:"annotationSortIndex"`
}

// Note is the note metadata and rich HTML returned by Zotero.
type Note struct {
	Key          string
	ParentKey    string
	DateAdded    string
	DateModified string
	Tags         []Tag
	HTML         string
}

type noteData struct {
	ItemType     string          `json:"itemType"`
	ParentItem   json.RawMessage `json:"parentItem"`
	DateAdded    string          `json:"dateAdded"`
	DateModified string          `json:"dateModified"`
	Tags         []Tag           `json:"tags"`
	Note         string          `json:"note"`
}

// Attachment is the metadata Zotero exposes for one attachment item.
type Attachment struct {
	Key          string
	ParentKey    string
	Title        string
	LinkMode     string
	ContentType  string
	Charset      string
	Filename     string
	URL          string
	AccessDate   string
	DateAdded    string
	DateModified string
	Tags         []Tag
	MD5          *string
	MTime        *int64
	Enclosure    *Link
}

// AttachmentFileStatus conservatively describes only metadata Zotero exposed.
type AttachmentFileStatus struct {
	State  string
	Reason string
}

type attachmentData struct {
	ItemType     string          `json:"itemType"`
	ParentItem   json.RawMessage `json:"parentItem"`
	Title        string          `json:"title"`
	LinkMode     string          `json:"linkMode"`
	ContentType  string          `json:"contentType"`
	Charset      string          `json:"charset"`
	Filename     string          `json:"filename"`
	URL          string          `json:"url"`
	AccessDate   string          `json:"accessDate"`
	DateAdded    string          `json:"dateAdded"`
	DateModified string          `json:"dateModified"`
	Tags         []Tag           `json:"tags"`
	MD5          json.RawMessage `json:"md5"`
	MTime        json.RawMessage `json:"mtime"`
}

type Creator struct {
	CreatorType string `json:"creatorType"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Name        string `json:"name"`
}

type Tag struct {
	Tag  string `json:"tag"`
	Type int    `json:"type"`
}

// CollectionData contains the stable collection fields used for tree rendering.
type CollectionData struct {
	Key              string          `json:"key"`
	Name             string          `json:"name"`
	ParentCollection json.RawMessage `json:"parentCollection"`
}

// ItemData decodes e.Data as Zotero item JSON.
func (e Envelope) ItemData() (ItemData, error) {
	var data ItemData
	if len(e.Data) == 0 {
		return data, nil
	}
	err := json.Unmarshal(e.Data, &data)
	return data, err
}

// Annotation decodes and validates the compact metadata for an annotation item.
func (e Envelope) Annotation() (Annotation, error) {
	if e.Key == "" {
		return Annotation{}, fmt.Errorf("missing annotation key")
	}
	if len(e.Data) == 0 {
		return Annotation{}, fmt.Errorf("annotation %s has no data", e.Key)
	}
	var data annotationData
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return Annotation{}, fmt.Errorf("annotation %s: %w", e.Key, err)
	}
	if data.ItemType != "annotation" {
		return Annotation{}, fmt.Errorf("item %s has type %q, not annotation", e.Key, data.ItemType)
	}
	if data.ParentItem == "" {
		return Annotation{}, fmt.Errorf("annotation %s has no parent attachment", e.Key)
	}
	if data.AnnotationType == "" {
		return Annotation{}, fmt.Errorf("annotation %s has no annotation type", e.Key)
	}
	return Annotation{
		Key:           e.Key,
		AttachmentKey: data.ParentItem,
		Type:          data.AnnotationType,
		PageLabel:     data.AnnotationPageLabel,
		Color:         data.AnnotationColor,
		SortIndex:     data.AnnotationSortIndex,
		HasText:       data.AnnotationText != "",
		HasComment:    data.AnnotationComment != "",
	}, nil
}

// Annotations decodes annotation envelopes in document order.
func Annotations(envelopes []Envelope) ([]Annotation, error) {
	annotations := make([]Annotation, 0, len(envelopes))
	for _, envelope := range envelopes {
		annotation, err := envelope.Annotation()
		if err != nil {
			return nil, err
		}
		annotations = append(annotations, annotation)
	}
	sort.Slice(annotations, func(i, j int) bool {
		left, right := annotations[i], annotations[j]
		if left.SortIndex == "" || right.SortIndex == "" {
			if left.SortIndex == "" && right.SortIndex != "" {
				return false
			}
			if left.SortIndex != "" && right.SortIndex == "" {
				return true
			}
		}
		if left.SortIndex == right.SortIndex {
			return left.Key < right.Key
		}
		return left.SortIndex < right.SortIndex
	})
	return annotations, nil
}

// Note decodes and validates one Zotero note item.
func (e Envelope) Note() (Note, error) {
	if e.Key == "" {
		return Note{}, fmt.Errorf("missing note key")
	}
	if len(e.Data) == 0 {
		return Note{}, fmt.Errorf("note %s has no data", e.Key)
	}
	var data noteData
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return Note{}, fmt.Errorf("note %s: %w", e.Key, err)
	}
	if data.ItemType != "note" {
		return Note{}, fmt.Errorf("item %s has type %q, not note", e.Key, data.ItemType)
	}
	parentKey, err := noteParentKey(data.ParentItem)
	if err != nil {
		return Note{}, fmt.Errorf("note %s parentItem: %w", e.Key, err)
	}
	tags := data.Tags
	if tags == nil {
		tags = []Tag{}
	}
	return Note{
		Key:          e.Key,
		ParentKey:    parentKey,
		DateAdded:    data.DateAdded,
		DateModified: data.DateModified,
		Tags:         tags,
		HTML:         data.Note,
	}, nil
}

// Notes decodes note envelopes in modified-descending, key-stable order.
func Notes(envelopes []Envelope) ([]Note, error) {
	notes := make([]Note, 0, len(envelopes))
	for _, envelope := range envelopes {
		note, err := envelope.Note()
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	sort.Slice(notes, func(i, j int) bool {
		if notes[i].DateModified == notes[j].DateModified {
			return notes[i].Key < notes[j].Key
		}
		return notes[i].DateModified > notes[j].DateModified
	})
	return notes, nil
}

func noteParentKey(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "false" {
		return "", nil
	}
	var key string
	if err := json.Unmarshal(raw, &key); err != nil {
		return "", err
	}
	return key, nil
}

// Attachment decodes and validates one Zotero attachment item.
func (e Envelope) Attachment() (Attachment, error) {
	if e.Key == "" {
		return Attachment{}, fmt.Errorf("missing attachment key")
	}
	if len(e.Data) == 0 {
		return Attachment{}, fmt.Errorf("attachment %s has no data", e.Key)
	}
	var data attachmentData
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return Attachment{}, fmt.Errorf("attachment %s: %w", e.Key, err)
	}
	if data.ItemType != "attachment" {
		return Attachment{}, fmt.Errorf("item %s has type %q, not attachment", e.Key, data.ItemType)
	}
	parentKey, err := attachmentParentKey(data.ParentItem)
	if err != nil {
		return Attachment{}, fmt.Errorf("attachment %s parentItem: %w", e.Key, err)
	}
	md5, err := nullableString(data.MD5)
	if err != nil {
		return Attachment{}, fmt.Errorf("attachment %s md5: %w", e.Key, err)
	}
	mtime, err := nullableInt64(data.MTime)
	if err != nil {
		return Attachment{}, fmt.Errorf("attachment %s mtime: %w", e.Key, err)
	}
	tags := data.Tags
	if tags == nil {
		tags = []Tag{}
	}
	var enclosure *Link
	if link, ok := e.Links["enclosure"]; ok {
		enclosure = &link
	}
	return Attachment{
		Key:          e.Key,
		ParentKey:    parentKey,
		Title:        data.Title,
		LinkMode:     data.LinkMode,
		ContentType:  data.ContentType,
		Charset:      data.Charset,
		Filename:     data.Filename,
		URL:          data.URL,
		AccessDate:   data.AccessDate,
		DateAdded:    data.DateAdded,
		DateModified: data.DateModified,
		Tags:         tags,
		MD5:          md5,
		MTime:        mtime,
		Enclosure:    enclosure,
	}, nil
}

// FileStatus interprets attachment metadata without claiming filesystem access.
func (a Attachment) FileStatus() AttachmentFileStatus {
	switch a.LinkMode {
	case "linked_url":
		return AttachmentFileStatus{State: "not-applicable", Reason: "attachment links to a URL and has no managed file"}
	case "linked_file":
		return AttachmentFileStatus{State: "linked-unverified", Reason: "the API provides no portable existence check for linked files"}
	case "imported_file", "imported_url":
		if a.Enclosure == nil {
			return AttachmentFileStatus{State: "unavailable", Reason: "Zotero did not advertise a file location"}
		}
		if a.Enclosure.Length != nil {
			return AttachmentFileStatus{State: "metadata-available", Reason: "Zotero advertised a file location and size metadata"}
		}
		return AttachmentFileStatus{State: "location-advertised", Reason: "Zotero advertised a file location without size metadata"}
	default:
		return AttachmentFileStatus{State: "unknown", Reason: "unrecognized attachment link mode"}
	}
}

func attachmentParentKey(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "false" {
		return "", nil
	}
	var key string
	if err := json.Unmarshal(raw, &key); err != nil {
		return "", err
	}
	return key, nil
}

func nullableString(raw json.RawMessage) (*string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func nullableInt64(raw json.RawMessage) (*int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err == nil {
		return &value, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, err
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

// CollectionData decodes e.Data as Zotero collection JSON.
func (e Envelope) CollectionData() (CollectionData, error) {
	var data CollectionData
	if len(e.Data) == 0 {
		return data, nil
	}
	err := json.Unmarshal(e.Data, &data)
	return data, err
}

// Relations returns an item's relation edges in stable predicate/target order.
func (e Envelope) Relations() ([]Relation, error) {
	if len(e.Data) == 0 {
		return []Relation{}, nil
	}
	var data struct {
		Relations map[string]json.RawMessage `json:"relations"`
	}
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return nil, err
	}
	relations := make([]Relation, 0)
	for predicate, rawTargets := range data.Relations {
		targets, err := relationTargets(rawTargets)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", predicate, err)
		}
		for _, target := range targets {
			relations = append(relations, Relation{
				Predicate: predicate,
				Target:    target,
				TargetKey: relationTargetKey(target),
			})
		}
	}
	sort.Slice(relations, func(i, j int) bool {
		if relations[i].Predicate == relations[j].Predicate {
			return relations[i].Target < relations[j].Target
		}
		return relations[i].Predicate < relations[j].Predicate
	})
	return relations, nil
}

// relationTargets accepts both relation shapes Zotero has exposed: current
// reads return an array, while the API's write examples use a single string.
func relationTargets(raw json.RawMessage) ([]string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty relation target")
	}
	switch trimmed[0] {
	case '"':
		var target string
		if err := json.Unmarshal(trimmed, &target); err != nil {
			return nil, err
		}
		return []string{target}, nil
	case '[':
		var targets []string
		if err := json.Unmarshal(trimmed, &targets); err != nil {
			return nil, err
		}
		return targets, nil
	default:
		return nil, fmt.Errorf("relation target must be a string or array of strings")
	}
}

func relationTargetKey(target string) string {
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host != "zotero.org" && host != "www.zotero.org" {
		return ""
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) != 4 || (parts[0] != "users" && parts[0] != "groups") || parts[2] != "items" {
		return ""
	}
	key, err := url.PathUnescape(parts[3])
	if err != nil {
		return ""
	}
	return key
}

// Title is a convenience accessor for item titles and collection names.
func (e Envelope) Title() string {
	if item, err := e.ItemData(); err == nil && item.Title != "" {
		return item.Title
	}
	if collection, err := e.CollectionData(); err == nil {
		return collection.Name
	}
	return ""
}

// ItemType returns data.itemType when this envelope wraps an item.
func (e Envelope) ItemType() string {
	item, err := e.ItemData()
	if err != nil {
		return ""
	}
	return item.ItemType
}

// CreatorSummary returns meta.creatorSummary.
func (e Envelope) CreatorSummary() string {
	return rawString(e.Meta["creatorSummary"])
}

// ParsedDate returns meta.parsedDate.
func (e Envelope) ParsedDate() string {
	return rawString(e.Meta["parsedDate"])
}

// NumChildren returns meta.numChildren. Missing/non-numeric values return 0.
func (e Envelope) NumChildren() int {
	raw, ok := e.Meta["numChildren"]
	if !ok {
		return 0
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		n, _ = strconv.Atoi(s)
	}
	return n
}

// ParentKey returns data.parentCollection as a collection key, or "" for top-level collections.
func (d CollectionData) ParentKey() string {
	var s string
	if err := json.Unmarshal(d.ParentCollection, &s); err == nil {
		return s
	}
	return ""
}

func rawString(raw json.RawMessage) string {
	var s string
	if len(raw) == 0 {
		return ""
	}
	_ = json.Unmarshal(raw, &s)
	return s
}
