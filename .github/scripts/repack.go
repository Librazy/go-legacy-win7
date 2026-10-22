// Repack turns a distpack archive into a go-legacy-win7 release archive.
//
// distpack writes pkg/distpack/go1.x.y.goos-goarch.tar.gz (or .zip) with a
// top-level go/ directory. This program drops the fork-only patches/ tree,
// renames that directory to go-legacy-win7/, and writes the archive under the
// name the existing releases use. test/, doc/, misc/, and *_test.go stay.
//
// The rewrite copies archive headers instead of rebuilding them from the
// filesystem, so modification times, modes, ownership, and entry order survive.
// The gzip header time is copied too; a zero value stays zero.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/flate"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	in := flag.String("in", "", "distpack archive")
	out := flag.String("out", "", "release archive")
	flag.Parse()
	if *in == "" || *out == "" || flag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "usage: repack -in distpack-archive -out release-archive\n")
		os.Exit(2)
	}

	var err error
	switch {
	case strings.HasSuffix(*in, ".tar.gz") && strings.HasSuffix(*out, ".tar.gz"):
		err = repackTar(*in, *out)
	case strings.HasSuffix(*in, ".zip") && strings.HasSuffix(*out, ".zip"):
		err = repackZip(*in, *out)
	default:
		err = fmt.Errorf("input and output must both be .tar.gz or both be .zip")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "repack: %v\n", err)
		os.Exit(1)
	}
}

func rename(name string) (string, bool) {
	const old = "go"
	if name != old && !strings.HasPrefix(name, old+"/") {
		return "", false
	}
	rest := strings.TrimPrefix(name, old)
	if rest == "/patches" || strings.HasPrefix(rest, "/patches/") {
		return "", false
	}
	return "go-legacy-win7" + rest, true
}

func repackTar(inName, outName string) error {
	in, err := os.Open(inName)
	if err != nil {
		return err
	}
	defer in.Close()
	gz, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer gz.Close()

	out, err := os.Create(outName)
	if err != nil {
		return err
	}
	defer out.Close()
	zw, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		return err
	}
	zw.Header = gz.Header
	zw.Header.Name = ""
	zw.Header.Comment = ""
	zw.Header.Extra = nil

	tr := tar.NewReader(gz)
	tw := tar.NewWriter(zw)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name, ok := rename(hdr.Name)
		if !ok {
			if _, err := io.Copy(io.Discard, tr); err != nil {
				return err
			}
			continue
		}
		hdr.Name = name
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := io.Copy(tw, tr); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return out.Close()
}

func repackZip(inName, outName string) error {
	zr, err := zip.OpenReader(inName)
	if err != nil {
		return err
	}
	defer zr.Close()

	out, err := os.Create(outName)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestCompression)
	})

	for _, f := range zr.File {
		name, ok := rename(f.Name)
		if !ok {
			continue
		}
		hdr := f.FileHeader
		hdr.Name = name
		w, err := zw.CreateRaw(&hdr)
		if err != nil {
			return err
		}
		r, err := f.OpenRaw()
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, r); err != nil {
			return err
		}
	}
	if comment := zr.Comment; comment != "" {
		if err := zw.SetComment(comment); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return out.Close()
}
