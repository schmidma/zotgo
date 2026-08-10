package zotero

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func annotationEnvelope(key, parent, annotationType, sortIndex, text, comment string) Envelope {
	data, _ := json.Marshal(map[string]any{
		"itemType":            "annotation",
		"parentItem":          parent,
		"annotationType":      annotationType,
		"annotationText":      text,
		"annotationComment":   comment,
		"annotationColor":     "#ffd400",
		"annotationPageLabel": "12",
		"annotationSortIndex": sortIndex,
		"annotationPosition":  `{"pageIndex":11}`,
	})
	return Envelope{Key: key, Data: data}
}

func TestEnvelopeAnnotation(t *testing.T) {
	annotation, err := annotationEnvelope("ANN00001", "ATTACH01", "highlight", "00012|00001", "quoted text", "comment").Annotation()
	if err != nil {
		t.Fatalf("Annotation: %v", err)
	}
	want := Annotation{
		Key:           "ANN00001",
		AttachmentKey: "ATTACH01",
		Type:          "highlight",
		PageLabel:     "12",
		Color:         "#ffd400",
		SortIndex:     "00012|00001",
		HasText:       true,
		HasComment:    true,
	}
	if !reflect.DeepEqual(annotation, want) {
		t.Fatalf("Annotation = %#v, want %#v", annotation, want)
	}
}

func TestAnnotationsDocumentOrder(t *testing.T) {
	envelopes := []Envelope{
		annotationEnvelope("ANN00003", "ATTACH01", "note", "", "", "note"),
		annotationEnvelope("ANN00002", "ATTACH01", "highlight", "00002", "text", ""),
		annotationEnvelope("ANN00001", "ATTACH01", "highlight", "00001", "text", ""),
		annotationEnvelope("ANN00000", "ATTACH01", "ink", "00001", "", ""),
	}
	annotations, err := Annotations(envelopes)
	if err != nil {
		t.Fatalf("Annotations: %v", err)
	}
	var keys []string
	for _, annotation := range annotations {
		keys = append(keys, annotation.Key)
	}
	want := []string{"ANN00000", "ANN00001", "ANN00002", "ANN00003"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("ordered keys = %v, want %v", keys, want)
	}
}

func TestAnnotationRejectsMalformedItems(t *testing.T) {
	tests := []struct {
		name string
		item Envelope
		want string
	}{
		{name: "missing key", item: Envelope{Data: json.RawMessage(`{"itemType":"annotation","parentItem":"ATTACH01","annotationType":"highlight"}`)}, want: "missing annotation key"},
		{name: "missing data", item: Envelope{Key: "ANN00001"}, want: "has no data"},
		{name: "invalid data", item: Envelope{Key: "ANN00001", Data: json.RawMessage(`{`)}, want: "unexpected end"},
		{name: "wrong type", item: Envelope{Key: "ANN00001", Data: json.RawMessage(`{"itemType":"note","parentItem":"ATTACH01","annotationType":"highlight"}`)}, want: `type "note"`},
		{name: "missing parent", item: Envelope{Key: "ANN00001", Data: json.RawMessage(`{"itemType":"annotation","annotationType":"highlight"}`)}, want: "no parent attachment"},
		{name: "missing annotation type", item: Envelope{Key: "ANN00001", Data: json.RawMessage(`{"itemType":"annotation","parentItem":"ATTACH01"}`)}, want: "no annotation type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.item.Annotation()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Annotation error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestAllChildItemsAndRawPreservePagination(t *testing.T) {
	const first = `{"key":"ANN00002","futureTop":{"kept":2},"data":{"itemType":"annotation","parentItem":"ATTACH01","annotationType":"highlight","annotationSortIndex":"00002","annotationText":"full text","annotationPosition":{"pageIndex":1}}}`
	const second = `{"key":"ANN00001","futureTop":{"kept":1},"data":{"itemType":"annotation","parentItem":"ATTACH01","annotationType":"note","annotationSortIndex":"00001","annotationComment":"full comment","futureData":true}}`

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/items/ATTACH01/children", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("itemType"); got != "annotation" {
			t.Errorf("itemType = %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "100" {
			t.Errorf("limit = %q", got)
		}
		if r.Header.Get("Zotero-API-Version") != "3" {
			t.Error("missing API version header")
		}
		switch r.URL.Query().Get("start") {
		case "":
			w.Header().Set("Total-Results", "2")
			w.Header().Set("Link", fmt.Sprintf(`<%s/api/users/0/items/ATTACH01/children?itemType=annotation&limit=100&start=1>; rel="next"`, srv.URL))
			_, _ = fmt.Fprintf(w, "[%s]", first)
		case "1":
			w.Header().Set("Total-Results", "2")
			_, _ = fmt.Fprintf(w, "[%s]", second)
		default:
			t.Fatalf("unexpected start %q", r.URL.Query().Get("start"))
		}
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	client := New(srv.URL)
	opts := ChildrenOptions{ItemType: "annotation", Limit: 100}
	items, err := client.AllChildItems(context.Background(), UserLibrary(), "ATTACH01", opts)
	if err != nil {
		t.Fatalf("AllChildItems: %v", err)
	}
	if len(items) != 2 || items[0].Key != "ANN00002" || items[1].Key != "ANN00001" {
		t.Fatalf("items = %#v", items)
	}

	raw, err := client.AllRawChildItems(context.Background(), UserLibrary(), "ATTACH01", opts)
	if err != nil {
		t.Fatalf("AllRawChildItems: %v", err)
	}
	if len(raw) != 2 {
		t.Fatalf("raw length = %d", len(raw))
	}
	for i, fixture := range []string{first, second} {
		var got, want any
		if err := json.Unmarshal(raw[i], &got); err != nil {
			t.Fatalf("decode raw[%d]: %v", i, err)
		}
		if err := json.Unmarshal([]byte(fixture), &want); err != nil {
			t.Fatalf("decode fixture[%d]: %v", i, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("raw[%d] changed envelope: got %#v, want %#v", i, got, want)
		}
	}
}

func TestAllRawChildItemsEmptyArray(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/items/ATTACH01/children", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	raw, err := New(srv.URL).AllRawChildItems(context.Background(), UserLibrary(), "ATTACH01", ChildrenOptions{ItemType: "annotation"})
	if err != nil {
		t.Fatalf("AllRawChildItems: %v", err)
	}
	if raw == nil || len(raw) != 0 {
		t.Fatalf("raw = %#v, want non-nil empty slice", raw)
	}
}
