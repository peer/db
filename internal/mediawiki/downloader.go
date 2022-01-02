package mediawiki

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	"gitlab.com/tozd/go/errors"
)

type Downloader struct {
	client     *retryablehttp.Client
	req        *retryablehttp.Request
	downloaded int64
	length     int64
	*http.Response
}

func (d *Downloader) Read(p []byte) (int, error) {
	n, err := d.Response.Body.Read(p)
	d.downloaded += int64(n)
	if d.downloaded == d.length {
		// We read everything, just return as-is.
		return n, err
	} else if d.downloaded > d.length {
		if err != nil {
			return n, errors.Wrap(err, "read beyond the expected end of the response body")
		}
		return n, errors.New("read beyond the expected end of the response body")
	} else if contextErr := d.req.Context().Err(); contextErr != nil {
		// Do not retry on context.Canceled or context.DeadlineExceeded.
		return n, contextErr
	} else if err != nil {
		// We have not read everything, but we got an error. We retry.
		errStart := d.start(d.downloaded)
		if errStart != nil {
			return n, errStart
		}
		if n > 0 {
			return n, nil
		}
		return d.Read(p)
	} else {
		// Something else, just return as-is.
		return n, err
	}
}

func (d *Downloader) Downloaded() int64 {
	return d.downloaded
}

func (d *Downloader) Length() int64 {
	return d.length
}

func (d *Downloader) Close() error {
	if d.Response != nil {
		err := errors.WithStack(d.Response.Body.Close())
		d.Response = nil
		return err
	}
	return nil
}

func (d *Downloader) start(from int64) errors.E {
	d.Close()
	if from <= 0 {
		d.req.Header.Del("Range")
	} else if from > 0 {
		d.req.Header.Set("Range", fmt.Sprintf("bytes=%d-", from))
	}
	resp, err := d.client.Do(d.req) //nolint:bodyclose
	if err != nil {
		return errors.WithStack(err)
	}
	if (from <= 0 && resp.StatusCode != http.StatusOK) || (from > 0 && resp.StatusCode != http.StatusPartialContent) {
		body, _ := io.ReadAll(resp.Body)
		return errors.Errorf("bad response status (%s): %s", resp.Status, strings.TrimSpace(string(body)))
	}
	d.Response = resp
	lengthStr := resp.Header.Get("Content-Length")
	if lengthStr == "" {
		return errors.Errorf("missing Content-Length header in response")
	}
	length, err := strconv.ParseInt(lengthStr, 10, 64) //nolint:gomnd
	if err != nil {
		return errors.WithStack(err)
	}
	if length == 0 {
		return errors.Errorf("Content-Length header in response is zero")
	}
	d.length = length
	return nil
}

func NewDownloader(client *retryablehttp.Client, req *retryablehttp.Request) (*Downloader, errors.E) {
	r := &Downloader{
		client:     client,
		req:        req,
		downloaded: 0,
		length:     0,
		Response:   nil,
	}
	err := r.start(0)
	if err != nil {
		return nil, err
	}
	return r, nil
}
