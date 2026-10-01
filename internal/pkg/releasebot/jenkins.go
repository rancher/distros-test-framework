package releasebot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Builder is what the scheduler needs from a Jenkins controller.
type Builder interface {
	Trigger(ctx context.Context, job *JenkinsJob) (queueURL string, err error)
	BuildFromQueue(ctx context.Context, queueURL string) (buildURL string, err error)
	Finished(ctx context.Context, buildURL string) (done bool, result string, err error)
}

type Jenkins struct {
	BaseURL string
	User    string
	Token   string
	HTTP    *http.Client
}

func NewJenkins(baseURL, user, token string) *Jenkins {
	return &Jenkins{
		BaseURL: strings.TrimRight(baseURL, "/"), User: user, Token: token,
		HTTP: &http.Client{Timeout: 60 * time.Second},
	}
}

// jobURL turns "distros_qa/rke2-tests/rke2_validate_cluster_qainfra" into ".../job/distros_qa/job/...".
func (j *Jenkins) jobURL(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		parts[i] = "job/" + url.PathEscape(p)
	}

	return j.BaseURL + "/" + strings.Join(parts, "/")
}

// Trigger calls buildWithParameters and returns the queue item URL from the Location header.
func (j *Jenkins) Trigger(ctx context.Context, job *JenkinsJob) (string, error) {
	req, err := j.triggerRequest(ctx, job)
	if err != nil {
		return "", err
	}

	// Never follow redirects on the trigger: the first hop may already have queued the build, and an
	// error on a later hop (e.g. a refused connection) would wrongly look like "never sent".
	noRedirect := *j.HTTP
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	// A canceled run sends nothing: Do would fail with an error that looks like "maybe sent".
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", fmt.Errorf("trigger %s: not sent: %w", job.Path, ctxErr)
	}

	resp, err := noRedirect.Do(req)
	if err != nil {
		if notSent(err) {
			return "", fmt.Errorf("trigger %s: %w", job.Path, err)
		}

		// The POST may have reached Jenkins (EOF, reset or timeout after sending): a build may exist.
		return "", fmt.Errorf("trigger %s: %w: %w", job.Path, ErrTriggerUnknown, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		err = fmt.Errorf("trigger %s: %s: %s", job.Path, resp.Status, strings.TrimSpace(string(msg)))
		switch {
		case resp.StatusCode == http.StatusBadGateway, resp.StatusCode == http.StatusServiceUnavailable,
			resp.StatusCode == http.StatusGatewayTimeout:
			// A proxy in front of Jenkins can answer this after Jenkins already queued the build.
			return "", fmt.Errorf("%w: %w", ErrTriggerUnknown, err)

		case resp.StatusCode >= 300 && resp.StatusCode < 400:
			// Something accepted the POST and redirected (scheme/host change, login page...):
			// whether a build was queued cannot be ruled out. Fix the controller URL.
			return "", fmt.Errorf("%w: %w (redirect to %q; check the controller URL)",
				ErrTriggerUnknown, err, resp.Header.Get("Location"))
		default:
			return "", err
		}
	}

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("trigger %s: %w: 201 without a queue Location", job.Path, ErrTriggerUnknown)
	}

	return loc, nil
}

// BuildFromQueue returns the build URL once the queue item has started, or "" while still queued.
func (j *Jenkins) BuildFromQueue(ctx context.Context, queueURL string) (string, error) {
	var q struct {
		Canceled   bool `json:"cancelled"` //nolint:misspell // Jenkins API field name
		Executable *struct {
			URL string `json:"url"`
		} `json:"executable"`
	}
	if err := j.getJSON(ctx, strings.TrimRight(queueURL, "/")+"/api/json", &q); err != nil {
		return "", err
	}
	if q.Canceled {
		return "", ErrQueueCanceled
	}
	if q.Executable == nil {
		return "", nil
	}

	return q.Executable.URL, nil
}

// Finished reports whether a build completed and its result.
func (j *Jenkins) Finished(ctx context.Context, buildURL string) (done bool, result string, err error) {
	var b struct {
		Building bool   `json:"building"`
		Result   string `json:"result"`
	}
	if getErr := j.getJSON(ctx, strings.TrimRight(buildURL, "/")+"/api/json?tree=building,result", &b); getErr != nil {
		return false, "", getErr
	}

	return !b.Building && b.Result != "", b.Result, nil
}

func (j *Jenkins) crumb(ctx context.Context) (field, value string, err error) {
	var c struct {
		Field string `json:"crumbRequestField"`
		Crumb string `json:"crumb"`
	}
	if err = j.getJSON(ctx, j.BaseURL+"/crumbIssuer/api/json", &c); err != nil {
		return "", "", nil //nolint:nilerr // controllers without CSRF protection have no crumb issuer
	}

	return c.Field, c.Crumb, nil
}

// notSent reports errors that prove the request never reached the server: DNS or connect failures.
func notSent(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var opErr *net.OpError

	return errors.As(err, &opErr) && opErr.Op == "dial"
}

// triggerRequest builds the buildWithParameters POST, with the crumb when Jenkins wants one.
func (j *Jenkins) triggerRequest(ctx context.Context, job *JenkinsJob) (*http.Request, error) {
	form := url.Values{}
	for k, v := range job.Params {
		form.Set(k, v)
	}

	crumbField, crumb, err := j.crumb(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.jobURL(job.Path)+"/buildWithParameters",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(j.User, j.Token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if crumbField != "" {
		req.Header.Set(crumbField, crumb)
	}

	return req, nil
}

// JobParams returns the parameter names a job defines; exists is false when Jenkins has no such
// job. Jenkins ignores parameters a job does not define, so a plan checks them before triggering.
func (j *Jenkins) JobParams(ctx context.Context, path string) (params []string, exists bool, err error) {
	u := j.jobURL(path) + "/api/json?tree=" + url.QueryEscape("property[parameterDefinitions[name]]")
	resp, err := j.get(ctx, u)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("GET %s: %s", u, resp.Status)
	}

	var out struct {
		Property []struct {
			ParameterDefinitions []struct {
				Name string `json:"name"`
			} `json:"parameterDefinitions"`
		} `json:"property"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, true, fmt.Errorf("GET %s: %w", u, err)
	}
	for _, p := range out.Property {
		for _, d := range p.ParameterDefinitions {
			params = append(params, d.Name)
		}
	}

	return params, true, nil
}

const (
	// maxConsole is how much of the end of a console log is kept: the failure is there.
	maxConsole = 8 << 20

	// maxConsoleRead bounds the stream; past it the end is not reached and the log is refused.
	maxConsoleRead = 512 << 20
)

// ConsoleText returns the end of a build's console log (its last 8 MiB), reading the whole
// stream so the tail is the real one; a log longer than 512 MiB is an error, not a wrong tail.
func (j *Jenkins) ConsoleText(ctx context.Context, buildURL string) (string, error) {
	u := strings.TrimRight(buildURL, "/") + "/consoleText"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(j.User, j.Token)

	// A long log streams for a while: the client's 60 s limit would cut it well before maxConsoleRead.
	client := *j.HTTP
	client.Timeout = consoleTimeout
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", u, resp.Status)
	}

	return consoleTail(resp.Body, maxConsole, maxConsoleRead)
}

// consoleTimeout bounds reading one console log (up to maxConsoleRead).
const consoleTimeout = 10 * time.Minute

// consoleTail keeps the last keep bytes of r, reading at most limit bytes. The buffer is trimmed
// only when it doubles, so copying stays proportional to the log size.
func consoleTail(r io.Reader, keep, limit int) (string, error) {
	tail := make([]byte, 0, 2*keep)
	buf := make([]byte, 64<<10)
	total := 0
	for {
		n, err := r.Read(buf)
		total += n
		if total > limit {
			return "", fmt.Errorf("console log longer than %d MiB", limit>>20)
		}
		tail = append(tail, buf[:n]...)
		if len(tail) > 2*keep {
			tail = append(tail[:0], tail[len(tail)-keep:]...)
		}
		if errors.Is(err, io.EOF) {
			if len(tail) > keep {
				tail = tail[len(tail)-keep:]
			}

			return string(tail), nil
		}
		if err != nil {
			return "", err
		}
	}
}

func (j *Jenkins) get(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(j.User, j.Token)

	return j.HTTP.Do(req)
}

func (j *Jenkins) getJSON(ctx context.Context, u string, out any) error {
	resp, err := j.get(ctx, u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", u, resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
