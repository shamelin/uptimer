package hooks

import (
	"context"
	"encoding/json"
	"github.com/google/go-github/v72/github"
	"gopkg.in/yaml.v3"
	"strings"
	"time"
	"uptimer/internal/seeking"
)

type HistoryType string

const (
	Investigating HistoryType = "investigating"
	Monitoring    HistoryType = "monitoring"
	Resolved      HistoryType = "resolved"
)

var commitMessage = "uptimer: update status"

type OutageYamlFile struct {
	section       string   `yaml:"section"`
	title         string   `yaml:"title"`
	date          string   `yaml:"date"`
	resolved      bool     `yaml:"resolved"`
	informational bool     `yaml:"informational"`
	resolvedWhen  string   `yaml:"resolvedWhen"`
	affected      []string `yaml:"affected"`
	severity      string   `yaml:"severity"`
}

type OutageHistoryFile struct {
	HistoryType HistoryType
	Description string
}

type Repository struct {
	Owner       string
	Name        string
	Branch      string
	CommitName  string
	CommitEmail string
}

type CStateHook struct {
	github  *github.Client
	repo    Repository
	outages []Outage
}

func NewCStateHook(githubToken string, repo Repository) *CStateHook {
	githubClient := github.NewClient(nil).WithAuthToken(githubToken)

	_, _, err := githubClient.Repositories.Get(context.Background(), repo.Owner, repo.Name)
	if err != nil {
		logger.Fatalf("Failed to get repository for CState hook: %v", err)
		panic(err)
	}

	cstateHook := &CStateHook{
		github:  githubClient,
		repo:    repo,
		outages: make([]Outage, 0),
	}
	// Load the state from the file if it exists
	err = cstateHook.loadState()
	if err != nil {
		logger.Errorf("Failed to load state: %v", err)
		return nil
	}

	return cstateHook
}

// Ensures an outage is created for the given list of application groups.
func (h *CStateHook) ensureOutages(appGroups []string) {
	for _, appGroup := range appGroups {
		outage := Outage{
			AppGroup:    appGroup,
			Hosts:       make([]OutageHost, 0),
			YamlContent: OutageYamlFile{section: appGroup},
		}
		h.outages = append(h.outages, outage)
	}
}

func (h *CStateHook) Handle(result SeekResult) error {
	//

	return nil
}

// handleOnline is called when the host is online. It updates the online status and
// dumps the state to the file if the status has changed.
func (h *CStateHook) handleOnline(result SeekResult) {
	for _, outage := range h.outages {
		outage.RemoveHost(OutageHost{
			Host:        result.Host.Host,
			Description: result.Host.OutageDescription,
			Severity:    result.Host.Severity,
		})
	}

	h.reprocessOutages()
}

// handleOffline is called when the host is offline. It updates the online status and
// dumps the state to the file if the status has changed.
func (h *CStateHook) handleOffline(result SeekResult) {
	outageHost := OutageHost{
		Host:        result.Host.Host,
		Description: result.Host.OutageDescription,
		Severity:    result.Host.Severity,
	}

	h.ensureOutages(result.Host.AppGroup)
	for _, outage := range h.outages {
		// Check if the outage app group matches one of the application groups
		if outage.RelatedToOutage(result.Host.AppGroup) {
			outage.AddHost(outageHost)
		}
	}

	h.reprocessOutages()
}

// Reprocesses the active outages and resolves them if all hosts are online.
func (h *CStateHook) reprocessOutages() {
	resolvedOutages := make([]int, 0)
	for index, outage := range h.outages {
		backOnline := len(outage.Hosts) == 0

		// Update the outage status in the repository
		h.updateOutageFile(outage)

		if backOnline {
			resolvedOutages = append(resolvedOutages, index)
		}
	}

	// Remove the resolved outages from the list
	for i := len(resolvedOutages) - 1; i >= 0; i-- {
		index := resolvedOutages[i]
		h.outages = append(h.outages[:index], h.outages[index+1:]...)
	}

	// Dump the state to the file
	err := h.dumpState()
	if err != nil {
		logger.Errorf("Failed to dump state: %v", err)
	}
}

// updateOutageFile updates the file of the outage and sets it as resolved.
func (h *CStateHook) updateOutageFile(outage Outage) {
	outageFile := outage.FormatOutageFile()

	// Update the file in the repository
	err := h.commit(outage.YamlContent.section, outageFile)
	if err != nil {
		logger.Errorf("Failed to commit file: [%v]", err)
		return
	}
}

// Dumps the content of the online statuses in the file `.uptimer.state` file at the root of
// the repository. This allows the application to pick up the state after a restart of the
// service.
func (h *CStateHook) dumpState() error {
	// Output each value in the "online" variable with a mapping key=value.
	fileContent := new(strings.Builder)

	for outage := range h.outages {
		encodedOutage, err := json.Marshal(outage)
		if err != nil {
			logger.Errorf("Failed to marshal outage: %v", err)
			return err
		}

		fileContent.WriteString(string(encodedOutage) + "\n")
	}

	// Write the file content to the `.uptimer.state` file in the root of the repository.
	err := h.commit(".uptimer.state", fileContent.String())
	if err != nil {
		logger.Errorf("Failed to commit file: [%v]", err)
		return err
	}

	logger.Infof("Dumped cstate state to file: [%s]", fileContent)
	return nil
}

// Reads the content of the `.uptimer.state` file in the root of the repository and
// updates the internal variables with the values from the file.
func (h *CStateHook) loadState() error {
	opts := &github.RepositoryContentGetOptions{
		Ref: h.repo.Branch,
	}
	fileContent, _, _, err := h.github.Repositories.GetContents(context.Background(), h.repo.Owner, h.repo.Name, ".uptimer.state", opts)
	if err != nil {
		logger.Errorf("Failed to get file content: [%v]", err)
		return err
	}

	content, err := fileContent.GetContent()
	if err != nil {
		logger.Errorf("Failed to get file content: [%v]", err)
		return err
	}

	lines := strings.Split(content, "\n")
	outages := make([]Outage, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}

		var outage Outage
		err := json.Unmarshal([]byte(line), &outage)
		if err != nil {
			logger.Errorf("Failed to unmarshal outage: [%v]", err)
			return err
		}

		outages = append(outages, outage)
	}

	logger.Infof("Loaded cstate state from file: [%s]", content)
	return nil
}

// Sends a commit to the repository with the content of the file. The commit message is
// set to "uptimer: update status" and the author is set to the configured commit name
// and email.
func (h *CStateHook) commit(file string, content string) error {
	// Get the SHA of the file to update
	getOpts := &github.RepositoryContentGetOptions{
		Ref: h.repo.Branch,
	}

	fileContent, _, _, err := h.github.Repositories.GetContents(context.Background(), h.repo.Owner, h.repo.Name, file, getOpts)
	if err != nil {
		logger.Errorf("Failed to get file content: [%v]", err)
		return err
	}

	// Get the SHA of the file to update
	fileSHA := fileContent.GetSHA()
	if fileSHA == "" {
		logger.Errorf("Failed to get SHA of file: [%s]", file)
		return err
	}

	// Now we can update the file with the new content
	updateOpts := &github.RepositoryContentFileOptions{
		Message: github.Ptr(commitMessage),
		Content: []byte(content),
		Branch:  github.Ptr(h.repo.Branch),
		SHA:     github.Ptr(fileSHA),
	}
	_, _, err = h.github.Repositories.UpdateFile(context.Background(), h.repo.Owner, h.repo.Name, file, updateOpts)
	if err != nil {
		return err
	}

	return nil
}

type OutageHost struct {
	Host        string           `json:"host"`
	Description string           `json:"description"`
	Severity    seeking.Severity `json:"severity"`
}

type Outage struct {
	AppGroup    string              `json:"app_group"`
	Hosts       []OutageHost        `json:"hosts"`
	YamlContent OutageYamlFile      `json:"yaml_content"`
	History     []OutageHistoryFile `json:"history"`
}

type OutageImpl interface {
	GetHighestSeverity() string
	FormatOutageFile() string
	RelatedToOutage([]string) bool
	AddHost(host OutageHost)
	RemoveHost(host OutageHost)
}

func (o *Outage) RelatedToOutage(appGroups []string) bool {
	for _, appGroup := range appGroups {
		if o.AppGroup == appGroup {
			return true
		}
	}
	return false
}

func (o *Outage) AddHost(host OutageHost) {
	o.Hosts = append(o.Hosts, host)
	o.History = append(o.History, OutageHistoryFile{
		HistoryType: Investigating,
		Description: "An additional sub-service related to the application group is experiencing issues. We are continuing to investigate the situation.",
	})
	o.YamlContent.severity = o.GetHighestSeverity()
}

func (o *Outage) RemoveHost(host OutageHost) {
	for i, h := range o.Hosts {
		if h.Host == host.Host {
			o.Hosts = append(o.Hosts[:i], o.Hosts[i+1:]...)
			o.History = append(o.History, OutageHistoryFile{
				HistoryType: Monitoring,
				Description: "One of the sub-services related to the application group is back online.",
			})

			// If the host is not in the outage anymore, we need to update the status and resolve the outage
			if len(o.Hosts) == 0 {
				o.YamlContent.resolved = true
				o.YamlContent.resolvedWhen = time.Now().UTC().Format(time.RFC3339)
				o.History = append(o.History, OutageHistoryFile{
					HistoryType: Resolved,
					Description: "We can now confirm that all services related to the service are back online. The outage is now considered resolved.",
				})
			}

			break
		}
	}
	o.YamlContent.severity = o.GetHighestSeverity()
}

func (o *Outage) GetHighestSeverity() string {
	highestSeverity := seeking.Minor
	for _, host := range o.Hosts {
		if int(host.Severity) > int(highestSeverity) {
			highestSeverity = host.Severity
		}
	}
	return highestSeverity.String()
}

func (o *Outage) FormatOutageFile() string {
	// Format the yaml
	yamlContent, err := yaml.Marshal(o.YamlContent)
	if err != nil {
		logger.Errorf("Failed to marshal yaml content: [%v]", err)
		return ""
	}

	// Format the history. We take each entrie and append it at the end
	historyContent := new(strings.Builder)
	for _, history := range o.History {
		historyContent.WriteString("**" + string(history.HistoryType) + "**" + ": " + history.Description + "\n")
	}

	// Format the outage file
	outageContent := new(strings.Builder)
	outageContent.WriteString("---\n")
	outageContent.Write(yamlContent)
	outageContent.WriteString("---\n")
	outageContent.WriteString(historyContent.String())

	return outageContent.String()
}
