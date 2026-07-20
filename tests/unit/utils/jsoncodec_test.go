package utils_test

import (
	"testing"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils/jsoncodec"
)

// TestJSONCodecMapEDecodesOrReturnsError 驗證 MapE 成功解碼並在失敗時回傳明確錯誤。
func TestJSONCodecMapEDecodesOrReturnsError(t *testing.T) {
	got, err := jsoncodec.MapE([]byte(`{"a":1,"b":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got["a"] != float64(1) || got["b"] != "x" {
		t.Fatalf("unexpected map decode: %+v", got)
	}
	if _, err := jsoncodec.MapE([]byte("{")); err == nil {
		t.Fatal("expected invalid JSON object to return an explicit error")
	}
	if got, err := jsoncodec.MapE(nil); err != nil || got != nil {
		t.Fatalf("empty input should decode to nil without error, got %+v err=%v", got, err)
	}
}

// TestJSONCodecPermissionsEDecodesOrReturnsError 驗證 PermissionsE 成功解碼並在失敗時回傳明確錯誤。
func TestJSONCodecPermissionsEDecodesOrReturnsError(t *testing.T) {
	got, err := jsoncodec.PermissionsE([]byte(`[{"resource":"agent","action":"read"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Resource != "agent" || got[0].Action != domain.Action("read") {
		t.Fatalf("unexpected permissions decode: %+v", got)
	}
	if _, err := jsoncodec.PermissionsE([]byte("{")); err == nil {
		t.Fatal("expected invalid JSON array to return an explicit error")
	}
	if got, err := jsoncodec.PermissionsE(nil); err != nil || got != nil {
		t.Fatalf("empty input should decode to nil without error, got %+v err=%v", got, err)
	}
}

// TestJSONCodecFailClosedVariantsUnchanged 驗證 Map/Permissions 維持 fail-closed 行為語義。
func TestJSONCodecFailClosedVariantsUnchanged(t *testing.T) {
	if got := jsoncodec.Map([]byte("{")); got != nil {
		t.Fatalf("expected invalid map payload to stay fail-closed nil, got %+v", got)
	}
	if got := jsoncodec.Permissions([]byte("{")); got != nil {
		t.Fatalf("expected invalid permissions payload to stay fail-closed nil, got %+v", got)
	}
	if got := jsoncodec.Map([]byte(`{"a":1}`)); got["a"] != float64(1) {
		t.Fatalf("expected valid map payload to decode, got %+v", got)
	}
	if got := jsoncodec.Permissions([]byte(`[{"resource":"agent","action":"read"}]`)); len(got) != 1 {
		t.Fatalf("expected valid permissions payload to decode, got %+v", got)
	}
}
