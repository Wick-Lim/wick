package cmd

import (
  "archive/tar"
  "bytes"
  "compress/gzip"
)

// makeTarGz creates a .tar.gz byte slice with given path->content entries.
func makeTarGz(files map[string][]byte) []byte {
  var buf bytes.Buffer
  gz := gzip.NewWriter(&buf)
  tw := tar.NewWriter(gz)
  for name, data := range files {
    _ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))})
    _, _ = tw.Write(data)
  }
  _ = tw.Close()
  _ = gz.Close()
  return buf.Bytes()
}

