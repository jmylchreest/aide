package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func TestDecisionApplicabilityRequestSchema(t *testing.T) {
	_, api := humatest.New(t)
	RegisterDecisionSchemas(api.OpenAPI().Components.Schemas)
	huma.Post(api, "/decisions", func(ctx context.Context, input *struct {
		Body memory.DecisionContext
	}) (*struct{ Body memory.DecisionContext }, error) {
		if err := input.Body.Validate("example"); err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return &struct{ Body memory.DecisionContext }{Body: input.Body}, nil
	})
	for _, body := range []string{
		`{}`, `{"applicability":null}`, `{"applicability":""}`,
		`{"applicability":"Only apps/admin/."}`,
		`{"applicability":{"kind":"project"}}`,
		`{"applicability":{"kind":"context","any_of":[{"paths":["apps/admin/**"]}]}}`,
	} {
		t.Run(body, func(t *testing.T) {
			resp := api.Post("/decisions", strings.NewReader(body))
			if resp.Code != http.StatusOK {
				t.Fatalf("%d: %s", resp.Code, resp.Body)
			}
			var want, got memory.DecisionContext
			if err := json.Unmarshal([]byte(body), &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("API changed context: %s", resp.Body)
			}
		})
	}
	for _, body := range []string{
		`{"applicability":42}`,
		`{"applicability":{"kind":"context","any_of":[{"path":["apps/admin/**"]}]}}`,
	} {
		resp := api.Post("/decisions", strings.NewReader(body))
		if resp.Code < 400 || resp.Code >= 500 {
			t.Errorf("accepted %s: %d", body, resp.Code)
		}
	}
}
