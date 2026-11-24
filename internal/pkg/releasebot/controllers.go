package releasebot

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type planResolver struct {
	gh      *gitHub
	jenkins map[string]*jenkins
}

func (r *planResolver) JobParams(ctx context.Context, controller, path string) (
	params []string, exists bool, err error,
) {
	j := r.jenkins[controller]
	if j == nil {
		return nil, false, fmt.Errorf("JENKINS_%s_AUTH is not set", strings.ToUpper(controller))
	}

	return j.JobParams(ctx, path)
}

func (r *planResolver) LatestGA(ctx context.Context, product, rc string) (version, note string, err error) {
	return r.gh.LatestGA(ctx, product, rc)
}

type controllerClient struct {
	name   string
	url    string
	client *jenkins
}

// Clients are reused for unchanged URLs and credentials; each plan still reloads its matrix.
func (a *App) controllerClients(m *Matrix) map[string]*jenkins {
	a.controllerMu.Lock()
	defer a.controllerMu.Unlock()
	if a.controllers == nil {
		a.controllers = map[string]controllerClient{}
	}

	clients := make(map[string]*jenkins, len(m.Controller))
	for name, lim := range m.Controller {
		baseURL := strings.TrimRight(lim.URL, "/")
		key := name + "|" + baseURL
		entry := controllerClient{name: name, url: baseURL}
		if user, token, ok := a.cfg.JenkinsAuth(name); ok {
			entry.client = a.controllers[key].client
			if entry.client == nil || entry.client.User != user || entry.client.Token != token {
				entry.client = a.cfg.NewJenkins(baseURL, user, token)
			}
			clients[name] = entry.client
		}
		a.controllers[key] = entry
	}
	a.controllersLoaded = true

	return clients
}

// buildersFor needs credentials only for the controllers jobs use: a Mower-only run needs no Baler token.
func buildersFor(m *Matrix, clients map[string]*jenkins, jobs []JenkinsJob) (map[string]Builder, error) {
	builders := make(map[string]Builder, len(m.Controller))
	for i := range jobs {
		name := jobs[i].Controller
		if _, known := m.Controller[name]; !known {
			continue // a saved build of a removed controller: the scheduler reports it as unknown
		}
		if clients[name] == nil {
			return nil, fmt.Errorf("set JENKINS_%s_AUTH=user:apitoken", strings.ToUpper(name))
		}
		builders[name] = clients[name]
	}

	return builders, nil
}

func (a *App) controllerSnapshot() (entries []controllerClient, loaded bool) {
	a.controllerMu.Lock()
	defer a.controllerMu.Unlock()
	for _, entry := range a.controllers {
		entries = append(entries, entry)
	}

	return entries, a.controllersLoaded
}

// JenkinsForBuild also serves builds from an older plan after the matrix changes.
func (a *App) JenkinsForBuild(buildURL string) (*jenkins, error) {
	entry, err := a.controllerForBuild(buildURL)
	if err != nil {
		return nil, err
	}

	return entry.authorized()
}

// TriageJenkins is JenkinsForBuild for -triage-build: a build the broker cannot triage is refused
// before its log is read or the agent runs.
func (a *App) TriageJenkins(buildURL string) (*jenkins, error) {
	entry, err := a.controllerForBuild(buildURL)
	if err != nil {
		return nil, err
	}

	if unsupportedErr := triageUnsupported(entry.name); unsupportedErr != nil {
		return nil, unsupportedErr
	}

	return entry.authorized()
}

func (a *App) controllerForBuild(buildURL string) (controllerClient, error) {
	entries, loaded := a.controllerSnapshot()
	if !loaded {
		m, err := a.loadMatrix()
		if err != nil {
			return controllerClient{}, err
		}
		a.controllerClients(m)
		entries, _ = a.controllerSnapshot()
	}

	for _, entry := range entries {
		if strings.HasPrefix(buildURL, entry.url+"/") {
			return entry, nil
		}
	}

	return controllerClient{}, fmt.Errorf("no controller in %s serves %s", a.cfg.MatrixPath, buildURL)
}

func (e *controllerClient) authorized() (*jenkins, error) {
	if e.client == nil {
		return nil, fmt.Errorf("JENKINS_%s_AUTH is not set", strings.ToUpper(e.name))
	}

	return e.client, nil
}

func (a *App) FailureTriage(spool string, wait time.Duration) *FailureTriage {
	return &FailureTriage{
		Log: func(ctx context.Context, buildURL string) (string, error) {
			j, err := a.JenkinsForBuild(buildURL)
			if err != nil {
				return "", err
			}

			return j.ConsoleText(ctx, buildURL)
		},
		Broker: &BrokerClient{Dir: spool, Timeout: wait, Poll: 5 * time.Second},
	}
}
