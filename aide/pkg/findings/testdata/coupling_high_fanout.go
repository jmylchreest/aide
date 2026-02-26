//go:build ignore

// Package fixture has an intentionally high number of imports for testing.
// This file is NOT compiled — it is used as test data for the coupling analyzer.
package fixture

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UseAllImports references every import so the file is syntactically valid Go.
func UseAllImports() {
	_ = bufio.NewReader(nil)
	_ = bytes.NewBuffer(nil)
	_ = &gzip.Reader{}
	_ = context.Background()
	_ = sha256.New()
	_ = base64.StdEncoding
	_ = json.NewEncoder(nil)
	_ = errors.New("")
	fmt.Println()
	_ = io.Discard
	log.Println()
	_ = &http.Client{}
	_ = os.Stderr
	_ = filepath.Join("")
	_ = regexp.MustCompile("")
	sort.Strings(nil)
	_ = strconv.Itoa(0)
	_ = strings.NewReader("")
	_ = &sync.Mutex{}
	_ = time.Now()
}
