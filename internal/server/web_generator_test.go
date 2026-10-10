package server

import (
	"net/http"
	"testing"
)

func TestWebGeneratorAlwaysGeneratesAndUsesTheModelOnlyWithAKey(t *testing.T) {
	gen, model, err := newWebGenerator(http.DefaultClient, "")
	if err != nil || gen == nil || model {
		t.Fatalf("without a key: generator %v, model %v, err %v", gen, model, err)
	}
	gen, model, err = newWebGenerator(http.DefaultClient, "sk-ant-example")
	if err != nil || gen == nil || !model {
		t.Fatalf("with a key: generator %v, model %v, err %v", gen, model, err)
	}
}
