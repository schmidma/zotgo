package zotero

import (
	"encoding/json"
	"fmt"
	"strconv"
)

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
