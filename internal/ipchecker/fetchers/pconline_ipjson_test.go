package fetchers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestPConlineIPJsonFetcher(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `if(window.IPCallBack) {IPCallBack({"ip":"91.103.122.87","pro":"","proCode":"999999","city":"","cityCode":"0","region":"","regionCode":"0","addr":" 乌克兰","regionNames":"","err":"noprovince"});}`
		gbkBytes, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(content))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(gbkBytes)
	}))
	defer server.Close()

	fetcher := &pconlineIPJsonFetcher{endpoint: server.URL}
	if fetcher.Name() != "pconline-ipjson" {
		t.Errorf("Expected name 'pconline-ipjson', got '%s'", fetcher.Name())
	}

	client := &http.Client{}

	ip, err := fetcher.Fetch(context.Background(), client)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if ip != "91.103.122.87" {
		t.Fatalf("unexpected ip: %s", ip)
	}
}

func TestExtractJSONObject(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "纯JSON响应",
			content:  `{"ip":"1.2.3.4","pro":"湖南","city":"长沙"}`,
			expected: `{"ip":"1.2.3.4","pro":"湖南","city":"长沙"}`,
		},
		{
			name:     "带var前缀的响应",
			content:  `var returnCitySN = {"ip":"1.2.3.4","pro":"湖南","city":"长沙"};`,
			expected: `{"ip":"1.2.3.4","pro":"湖南","city":"长沙"}`,
		},
		{
			name:     "带if语句的响应",
			content:  `if(window.returnCitySN){}else{var returnCitySN = {"ip":"1.2.3.4","pro":"湖南"};}`,
			expected: `{"ip":"1.2.3.4","pro":"湖南"}`,
		},
		{
			name:     "无JSON对象",
			content:  `no json here`,
			expected: "",
		},
		{
			name:     "带空格的JSON",
			content:  `{ "ip" : "1.2.3.4" }`,
			expected: `{ "ip" : "1.2.3.4" }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSONObject(tt.content)
			if result != tt.expected {
				t.Errorf("extractJSONObject(%q) = %q, want %q", tt.content, result, tt.expected)
			}
		})
	}
}

func TestExtractJSONObjectShouldWorkWithCallbackWrapper(t *testing.T) {
	t.Parallel()

	content := `if(window.IPCallBack) {IPCallBack({"ip":"1.2.3.4","pro":"","city":"","addr":" 乌克兰"});}`
	got := extractJSONObject(content)
	want := `{"ip":"1.2.3.4","pro":"","city":"","addr":" 乌克兰"}`
	if got != want {
		t.Fatalf("unexpected: %s", got)
	}
}
