// Copyright © 2020 The Knative Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package profiling

import (
	"bytes"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"gotest.tools/assert"
)

func TestProfileDownload(t *testing.T) {
	t.Run("download heap profile success", func(t *testing.T) {
		downloadData := []byte("some-binary-data")
		server := httptest.NewServer(http.HandlerFunc(
			func(rw http.ResponseWriter, req *http.Request) {
				assert.Equal(t, "/debug/pprof/heap", req.URL.RequestURI())
				rw.WriteHeader(http.StatusOK)
				rw.Write(downloadData)
			},
		))
		defer server.Close()

		listenerAddr := server.Listener.Addr().String()
		_, portString, err := net.SplitHostPort(listenerAddr)
		assert.NilError(t, err)
		port, err := strconv.ParseInt(portString, 10, 0)
		assert.NilError(t, err)

		d := &Downloader{
			readyCh:   make(chan struct{}),
			stopCh:    make(chan struct{}),
			client:    http.DefaultClient,
			localPort: uint32(port),
		}
		errChan := make(chan error)
		output := &bytes.Buffer{}
		go func() {
			errChan <- d.Download(ProfileTypeHeap, output)
		}()
		close(d.readyCh)

		err = <-errChan
		assert.NilError(t, err)

		bs, err := ioutil.ReadAll(output)
		assert.NilError(t, err)
		assert.DeepEqual(t, downloadData, bs)
	})

	t.Run("download error caused by response code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(
			func(rw http.ResponseWriter, req *http.Request) {
				rw.WriteHeader(http.StatusNotFound)
				io.WriteString(rw, "not found")
			},
		))
		defer server.Close()

		listenerAddr := server.Listener.Addr().String()
		_, portString, err := net.SplitHostPort(listenerAddr)
		assert.NilError(t, err)
		port, err := strconv.ParseInt(portString, 10, 0)
		assert.NilError(t, err)

		d := &Downloader{
			readyCh:   make(chan struct{}),
			stopCh:    make(chan struct{}),
			client:    http.DefaultClient,
			localPort: uint32(port),
		}
		errChan := make(chan error)
		output := &bytes.Buffer{}
		go func() {
			errChan <- d.Download(ProfileTypeHeap, output)
		}()
		close(d.readyCh)

		err = <-errChan
		assert.ErrorContains(t, err, "download error: not found, code 404")
	})

	t.Run("unsupported profile type", func(t *testing.T) {

		d := &Downloader{
			readyCh: make(chan struct{}),
			stopCh:  make(chan struct{}),
			client:  http.DefaultClient,
		}
		errChan := make(chan error)
		output := &bytes.Buffer{}

		go func() {
			errChan <- d.Download(ProfileTypeUnknown, output)
		}()
		close(d.readyCh)
		var err error
		err = <-errChan
		assert.ErrorContains(t, err, "unsupported profiling type")

		go func() {
			errChan <- d.Download(ProfileType(len(ProfileEndpoints)), output)
		}()
		err = <-errChan
		assert.ErrorContains(t, err, "unsupported profiling type")
	})

	t.Run("request canceled while download is not started", func(t *testing.T) {
		d := &Downloader{
			readyCh: make(chan struct{}),
			stopCh:  make(chan struct{}),
			client:  http.DefaultClient,
		}
		errChan := make(chan error)
		output := &bytes.Buffer{}

		go func() {
			errChan <- d.Download(ProfileTypeHeap, output)
		}()
		close(d.stopCh)
		var err error
		err = <-errChan
		assert.ErrorContains(t, err, "download failed")
	})

	t.Run("request canceled while download is started", func(t *testing.T) {
		downloadData := []byte("some-binary-data")
		server := httptest.NewServer(http.HandlerFunc(
			func(rw http.ResponseWriter, req *http.Request) {
				rw.WriteHeader(http.StatusOK)
				for _, b := range downloadData {
					<-time.After(time.Second) // write at 1 byte/second
					rw.Write([]byte{b})
				}
			},
		))
		defer server.Close()

		listenerAddr := server.Listener.Addr().String()
		_, portString, err := net.SplitHostPort(listenerAddr)
		assert.NilError(t, err)
		port, err := strconv.ParseInt(portString, 10, 0)
		assert.NilError(t, err)

		d := &Downloader{
			readyCh:   make(chan struct{}),
			stopCh:    make(chan struct{}),
			client:    http.DefaultClient,
			localPort: uint32(port),
		}
		errChan := make(chan error)
		output := &bytes.Buffer{}
		go func() {
			errChan <- d.Download(ProfileTypeHeap, output)
		}()
		close(d.readyCh)
		<-time.After(1 * time.Second)
		close(d.stopCh)

		err = <-errChan
		assert.ErrorContains(t, err, "context canceled")
	})
}
