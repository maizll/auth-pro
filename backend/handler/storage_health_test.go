package handler

import (
	"errors"
	"testing"
)

// TestHealthSentenceNoDoublePeriod 核对存储检查说明不出现「。。」：自带句号的错误不再补，没带的补上。
func TestHealthSentenceNoDoublePeriod(t *testing.T) {
	cases := map[string]string{
		githubPaidPublicRepoText: githubPaidPublicRepoText,
		"连接超时":                   "连接超时。",
		"dial tcp: timeout.":     "dial tcp: timeout。",
	}
	for in, want := range cases {
		if got := healthSentence(errors.New(in)); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}
