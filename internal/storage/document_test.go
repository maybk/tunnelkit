package storage

import (
	"encoding/json"
	"testing"
)

func TestDocumentVersionBelongsToItsOwner(t *testing.T) {
	raw, err := MarshalDocument(2, []string{"value"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := DecodeDocument(raw, 1); err == nil {
		t.Fatal("different owner format accepted")
	}
	payload, legacy, err := DecodeDocument(raw, 2)
	if err != nil || legacy {
		t.Fatal("current version failed", err)
	}
	var got []string
	if err := json.Unmarshal(payload, &got); err != nil || len(got) != 1 || got[0] != "value" {
		t.Fatal("payload changed", err)
	}
}

func TestDocumentRecognizesOnlyUnversionedLegacyData(t *testing.T) {
	for _, raw := range []string{`[]`, `{"profile-id":{"nonce":"synthetic","ciphertext":"synthetic"}}`} {
		payload, legacy, err := DecodeDocument([]byte(raw), 1)
		if err != nil || !legacy || string(payload) != raw {
			t.Fatal("legacy format changed", err)
		}
	}
	for _, raw := range []string{`{"schemaVersion":0,"data":[]}`, `{"schemaVersion":null,"data":[]}`, `{"schemaVersion":1}`, `{"schemaVersion":1,"data":null}`} {
		if _, _, err := DecodeDocument([]byte(raw), 1); err == nil {
			t.Fatal("invalid versioned document accepted")
		}
	}
}

func TestDocumentWriterRejectsMissingPayload(t *testing.T) {
	for _, value := range []any{nil, ([]string)(nil), (map[string]string)(nil)} {
		if _, err := MarshalDocument(1, value); err == nil {
			t.Fatal("writer produced a document the reader cannot load")
		}
	}
}
