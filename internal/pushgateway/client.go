package pushgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/opentracing/opentracing-go"
)

type httpClient struct {
	hc *http.Client
}

// NewHTTPClient creates a new Push Gateway client that uses an
// *http.Client.
func NewHTTPClient(hc *http.Client) Client {
	return &httpClient{hc: hc}
}

func (h *httpClient) Notify(ctx context.Context, url string, req *NotifyRequest, resp *NotifyResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "Notify")
	defer span.Finish()

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	hreq.Header.Set("Content-Type", "application/json")

	hresp, err := h.hc.Do(hreq)
	if err != nil {
		return err
	}
	defer hresp.Body.Close()

	if hresp.StatusCode == http.StatusOK {
		return json.NewDecoder(hresp.Body).Decode(resp)
	}

	var errorBody struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(hresp.Body).Decode(&errorBody); err == nil {
		return fmt.Errorf("push gateway: %d from %s: %s", hresp.StatusCode, url, errorBody.Message)
	}
	return fmt.Errorf("push gateway: %d from %s", hresp.StatusCode, url)
}
