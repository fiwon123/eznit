package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

type payload struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// The envelope is the wire contract every handler answers with, so its field
// names and its omitempty behaviour are the contract, not an implementation
// detail: a client decodes `data` and reads `message`/`error`/`status_code`.
func TestEnvelopeJSON(t *testing.T) {
	tests := []struct {
		name string
		body any
		want string
	}{
		{
			name: "all fields set",
			body: Envelope[payload]{
				StatusCode: 200,
				Data:       payload{ID: 1, Name: "a.txt"},
				Msg:        "ok",
				Timestamp:  "2026-11-09T00:00:00Z",
			},
			want: `{"status_code":200,"data":{"id":1,"name":"a.txt"},"message":"ok","timestamp":"2026-11-09T00:00:00Z"}`,
		},
		{
			name: "error instead of data",
			body: Envelope[any]{StatusCode: 400, Error: "invalid credentials"},
			want: `{"status_code":400,"error":"invalid credentials"}`,
		},
		{
			name: "zero value is an empty object",
			body: Envelope[any]{},
			want: `{}`,
		},
		{
			name: "nil slice is omitted",
			body: Envelope[[]string]{StatusCode: 200},
			want: `{"status_code":200}`,
		},
		{
			name: "empty slice is omitted",
			body: Envelope[[]string]{StatusCode: 200, Data: []string{}},
			want: `{"status_code":200}`,
		},
		{
			name: "non-empty slice is kept",
			body: Envelope[[]string]{StatusCode: 200, Data: []string{"a", "b"}},
			want: `{"status_code":200,"data":["a","b"]}`,
		},
		{
			// The interface is non-nil, so omitempty keeps it and the empty
			// array reaches the client. That is what a caller building an
			// Envelope[any] around a list already gets; this pins it.
			name: "empty slice behind an interface is kept",
			body: Envelope[any]{StatusCode: 200, Data: []string{}},
			want: `{"status_code":200,"data":[]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Marshal() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	want := Envelope[payload]{
		StatusCode: 201,
		Data:       payload{ID: 7, Name: "report.pdf"},
		Msg:        "created",
		Timestamp:  "2026-11-09T12:00:00Z",
	}

	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got Envelope[payload]
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

// A body written by an older or newer client may carry a subset of the
// fields; the missing ones must decode to their zero value, not fail.
func TestEnvelopePartialDecode(t *testing.T) {
	var got Envelope[payload]

	if err := json.Unmarshal([]byte(`{"data":{"id":3},"message":"found"}`), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	want := Envelope[payload]{Data: payload{ID: 3}, Msg: "found"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("decoded = %+v, want %+v", got, want)
	}
}
