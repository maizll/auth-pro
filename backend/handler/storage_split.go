package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// shardManifest 记在对象存储里，下载时按 parts 顺序拼回原文件再核对 sha256。
// 买家和目录不需要知道文件被切开了。
type shardManifest struct {
	V      int      `json:"v"`
	SHA256 string   `json:"sha256"`
	Size   int64    `json:"size"`
	Parts  []string `json:"parts"`
}

func splitPayload(payload []byte, partSize int64) ([][]byte, error) {
	if partSize <= 0 {
		return nil, errors.New("分片大小不合法")
	}
	if int64(len(payload)) <= partSize {
		return [][]byte{payload}, nil
	}
	out := make([][]byte, 0, int(int64(len(payload))/partSize)+1)
	for offset := 0; offset < len(payload); offset += int(partSize) {
		end := offset + int(partSize)
		if end > len(payload) {
			end = len(payload)
		}
		out = append(out, payload[offset:end])
	}
	if len(out) > 10000 {
		return nil, errors.New("文件过大，分片数量超过 10000")
	}
	return out, nil
}

func joinShards(parts [][]byte, expectedSHA string, expectedSize int64) ([]byte, error) {
	total := 0
	for _, part := range parts {
		total += len(part)
	}
	if expectedSize > 0 && int64(total) != expectedSize {
		return nil, errors.New("分片拼合后的大小和清单不一致")
	}
	buf := make([]byte, 0, total)
	for _, part := range parts {
		buf = append(buf, part...)
	}
	sum := sha256.Sum256(buf)
	got := hex.EncodeToString(sum[:])
	if expectedSHA != "" && !strings.EqualFold(got, expectedSHA) {
		return nil, errors.New("分片拼合后的校验码不一致")
	}
	return buf, nil
}

func marshalShardManifest(sha string, size int64, parts []string) ([]byte, error) {
	payload, err := json.Marshal(shardManifest{V: 1, SHA256: strings.ToLower(sha), Size: size, Parts: parts})
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func parseShardManifest(payload []byte) (shardManifest, bool) {
	if len(payload) == 0 || payload[0] != '{' {
		return shardManifest{}, false
	}
	var manifest shardManifest
	if err := json.Unmarshal(payload, &manifest); err != nil || manifest.V != 1 || len(manifest.Parts) == 0 || len(manifest.SHA256) != 64 {
		return shardManifest{}, false
	}
	return manifest, true
}

func shardPartName(base string, index int) string {
	return fmt.Sprintf("%s.part%03d", base, index+1)
}

func shardManifestName(base string) string {
	return base + ".parts.json"
}
