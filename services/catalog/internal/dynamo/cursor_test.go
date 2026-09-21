package dynamo

import (
	"encoding/base64"
	"errors"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/Xtazhoxton/monster-arena/services/catalog/internal/catalog"
)

func TestCursorRoundTrip(t *testing.T) {
	key := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: creaturePK("pikachu")},
		"SK": &types.AttributeValueMemberS{Value: creatureEntity},
	}

	token, err := encodeCursor(key)
	if err != nil {
		t.Fatalf("encodeCursor() = %v, want nil", err)
	}

	back, err := decodeCursor(token)
	if err != nil {
		t.Fatalf("decodeCursor(%q) = %v, want nil", token, err)
	}

	if !reflect.DeepEqual(back, key) {
		t.Errorf("round trip gave %v, want %v", back, key)
	}
}

func TestCursorEdges(t *testing.T) {
	if token, err := encodeCursor(nil); token != "" || err != nil {
		t.Errorf("encodeCursor(nil) = (%q, %v), want (\"\", nil)", token, err)
	}

	if key, err := decodeCursor(""); key != nil || err != nil {
		t.Errorf("decodeCursor(\"\") = (%v, %v), want (nil, nil)", key, err)
	}
}

func TestDecodeCursorRejectsBadTokens(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{name: "not base64", token: "pas du base64 !"},
		{name: "not json", token: base64.RawURLEncoding.EncodeToString([]byte("hello"))},
		{name: "empty object", token: base64.RawURLEncoding.EncodeToString([]byte(`{}`))},
		{name: "partition key only", token: base64.RawURLEncoding.EncodeToString([]byte(`{"pk":"CREATURE#pikachu"}`))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeCursor(tt.token); !errors.Is(err, catalog.ErrInvalid) {
				t.Errorf("decodeCursor(%q) = %v, want an error wrapping ErrInvalid", tt.token, err)
			}
		})
	}
}
