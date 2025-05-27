package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/go-github/v72/github"
	"gopkg.in/yaml.v3"
	"strconv"
	"strings"
	"time"
	"uptimer/internal/seeking/dto"
)

type HistoryType string

const (
	Investigating HistoryType = "Investigating"
	Monitoring    HistoryType = "Monitoring"
	Resolved      HistoryType = "Resolved"
)

var statusCommitMessage = "uptimer: update status"
var stateCommitMessage = "uptimer: update state"
var skipDeploymentPrefix = "[CF-Pages-Skip]"

type OutageYamlFile struct {
	Section       string   `yaml:"section"`
	Title         string   `yaml:"title"`
	Date          string   `yaml:"date"`
	Resolved      bool     `yaml:"resolved"`
	Informational bool     `yaml:"informational"`
	ResolvedWhen  string   `yaml:"resolvedWhen"`
	Affected      []string `yaml:"affected"`
	Severity      string   `yaml:"severity"`
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
	deploymentInterval time.Duration
	github             *github.Client
	repo               Repository
	outages            []Outage
	downMapping        map[string]int
}

func NewCStateHook(githubToken string, repo Repository) *CStateHook {
	githubClient := github.NewClient(nil).WithAuthToken(githubToken)

	_, _, err := githubClient.Repositories.Get(context.Background(), repo.Owner, repo.Name)
	if err != nil {
		logger.Fatalf("Failed to get repository for CState hook: %v", err)
		panic(err)
	}

	cstateHook := &CStateHook{
		github:      githubClient,
		repo:        repo,
		outages:     make([]Outage, 0),
		downMapping: make(map[string]int),
	}
	// Load the state from the file if it exists
	err = cstateHook.loadState()
	if err != nil {
		logger.Errorf("Failed to load state, will start from scratch: [%v]", err)
	}

	return cstateHook
}

// Ensures an outage is created for the given list of application groups.
func (h *CStateHook) ensureOutages(affectedAppGroups []string) {
	for index := range h.outages {
		if h.outages[index].RelatedToOutage(affectedAppGroups) {
			return
		}
	}

	// set the filename to the current date and time
	fileName := new(strings.Builder)
	fileName.WriteString("content/issues/")
	fileName.WriteString(time.Now().UTC().Format("2006-01-02-150405"))
	fileName.WriteString("-")
	for index := range affectedAppGroups {
		fileName.WriteString(affectedAppGroups[index])
		if index != len(affectedAppGroups)-1 {
			fileName.WriteString("-")
		}
	}
	fileName.WriteString(".md")

	// create initial outage message
	outageHistory := OutageHistoryFile{
		HistoryType: Investigating,
		Description: "An issue has been detected with one or more service. We are currently investigating the situation and will provide updates when possible.",
	}

	currentDate := time.Now().UTC()
	title := "Outage related to " + strconv.Itoa(len(affectedAppGroups)) + " service"
	if len(affectedAppGroups) > 1 {
		title += "s"
	}
	outage := Outage{
		Filename:  fileName.String(),
		AppGroups: affectedAppGroups,
		Hosts:     make([]OutageHost, 0),
		YamlContent: OutageYamlFile{
			Section:  "issue",
			Title:    title,
			Affected: affectedAppGroups,
			Date:     currentDate.Format("2006-01-02 15:04:05"),
		},
		History: []OutageHistoryFile{
			outageHistory,
		},
	}
	h.outages = append(h.outages, outage)
}

func (h *CStateHook) Handle(result SeekResult) error {
	if len(result.Host.AppGroup) == 0 {
		return nil
	}

	if result.Online {
		return h.handleOnline(result)
	} else {
		return h.handleOffline(result)
	}
}

// handleOnline is called when the host is online. It updates the online status and
// dumps the state to the file if the status has changed.
func (h *CStateHook) handleOnline(result SeekResult) error {
	delete(h.downMapping, result.Host.Host)

	for i := range h.outages {
		h.outages[i].RemoveHost(OutageHost{
			Host:        result.Host.Host,
			Description: result.Host.OutageDescription,
			Severity:    result.Host.Severity,
		})
	}

	return h.reprocessOutages()
}

// handleOffline is called when the host is offline. It updates the online status and
// dumps the state to the file if the status has changed.
func (h *CStateHook) handleOffline(result SeekResult) error {
	if _, ok := h.downMapping[result.Host.Host]; !ok {
		// If the host is not in the mapping, we need to create a new outage
		h.downMapping[result.Host.Host] = 1
		return nil
	} else {
		// If the host is already in the mapping, we increment the count
		h.downMapping[result.Host.Host]++
		if h.downMapping[result.Host.Host] < result.Host.OutageDownThreshold {
			return nil
		}
	}

	outageHost := OutageHost{
		Host:        result.Host.Host,
		Description: result.Host.OutageDescription,
		Severity:    result.Host.Severity,
	}

	h.ensureOutages(result.Host.AppGroup)
	for index := range h.outages {
		outage := &h.outages[index]
		// Check if the outage app group matches one of the application groups
		if outage.RelatedToOutage(result.Host.AppGroup) && !outage.HostExists(outageHost) {
			outage.AddHost(outageHost)
		}
	}

	return h.reprocessOutages()
}

// Reprocesses the active outages and resolves them if all hosts are online.
func (h *CStateHook) reprocessOutages() error {
	resolvedOutages := make([]int, 0)
	for index := range h.outages {
		backOnline := len(h.outages[index].Hosts) == 0

		// Update the outage status in the repository
		h.updateOutageFile(h.outages[index])

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
		return err
	}

	return nil
}

// updateOutageFile updates the file of the outage and sets it as resolved.
func (h *CStateHook) updateOutageFile(outage Outage) {
	outageFile := outage.FormatOutageFile()

	// Update the file in the repository
	err := h.commit(outage.Filename, outageFile, statusCommitMessage, false)
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

	for index := range h.outages {
		encodedOutage, err := json.Marshal(h.outages[index])
		if err != nil {
			logger.Errorf("Failed to marshal outage: %v", err)
			return err
		}

		fileContent.WriteString(string(encodedOutage) + "\n")
	}

	// Write the file content to the `.uptimer.state` file in the root of the repository.
	err := h.commit(".uptimer.state", fileContent.String(), stateCommitMessage, true)
	if err != nil {
		logger.Errorf("Failed to commit file: [%v]", err)
		return err
	}

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
	for index := range lines {
		if lines[index] == "" {
			continue
		}

		var outage Outage
		err := json.Unmarshal([]byte(lines[index]), &outage)
		if err != nil {
			logger.Errorf("Failed to unmarshal outage: [%v]", err)
			return err
		}

		outages = append(outages, outage)
	}

	h.outages = outages

	logger.Infof("Loaded cstate state from file: [%s]", content)
	return nil
}

// Sends a commit to the repository with the content of the file. The commit message is
// set to "uptimer: update status" and the author is set to the configured commit name
// and email.
func (h *CStateHook) commit(file string, content string, commit string, skipci bool) error {
	// Check if the file exists
	getOpts := &github.RepositoryContentGetOptions{
		Ref: h.repo.Branch,
	}

	formattedCommit := commit
	if skipci {
		formattedCommit = skipDeploymentPrefix + " " + commit
	}

	fileContent, _, _, err := h.github.Repositories.GetContents(context.Background(), h.repo.Owner, h.repo.Name, file, getOpts)
	if err != nil {
		var errorResponse *github.ErrorResponse
		if errors.As(err, &errorResponse) && errorResponse.Response.StatusCode == 404 {
			// File does not exist, create it
			return h.createFile(file, content, formattedCommit)
		}
		logger.Errorf("Failed to get file content: [%v]", err)
		return err
	}

	// Only update the file if the content is different
	existingContent, err := fileContent.GetContent()
	if err != nil {
		logger.Errorf("Failed to get file content: [%v]", err)
	}

	if existingContent == content {
		return nil
	}

	return h.updateFile(file, content, formattedCommit)
}

func (h *CStateHook) createFile(file string, content string, commit string) error {
	createOpts := &github.RepositoryContentFileOptions{
		Message: github.Ptr(commit),
		Content: []byte(content),
		Branch:  github.Ptr(h.repo.Branch),
	}
	_, _, err := h.github.Repositories.CreateFile(context.Background(), h.repo.Owner, h.repo.Name, file, createOpts)
	if err != nil {
		return err
	}

	return nil
}

func (h *CStateHook) updateFile(file string, content string, commit string) error {
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

	updateOpts := &github.RepositoryContentFileOptions{
		Message: github.Ptr(commit),
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
	Host        string       `json:"host"`
	Description string       `json:"description"`
	Severity    dto.Severity `json:"severity"`
}

type Outage struct {
	Filename    string              `json:"filename"`
	AppGroups   []string            `json:"app_groups"`
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
	if len(appGroups) != len(o.AppGroups) {
		return false
	}

	for index := range appGroups {
		if appGroups[index] != o.AppGroups[index] {
			return false
		}
	}

	return true
}

func (o *Outage) HostExists(host OutageHost) bool {
	for index := range o.Hosts {
		if o.Hosts[index].Host == host.Host {
			return true
		}
	}
	return false
}

func (o *Outage) getCurrentlyAffectedMessage() string {
	if len(o.Hosts) == 0 {
		return "All services related to outage are considered back online."
	}

	description := new(strings.Builder)
	description.WriteString(strconv.Itoa(len(o.Hosts)))
	description.WriteByte(' ')
	if len(o.Hosts) == 1 {
		description.WriteString("sub-service is currently affected by this outage.")
	} else {
		description.WriteString("sub-services are currently affected by this outage.")
	}

	return description.String()
}

func (o *Outage) AddHost(host OutageHost) {
	o.Hosts = append(o.Hosts, host)

	description := new(strings.Builder)
	if host.Description != "" {
		description.WriteString("A service that " + host.Description + " is experiencing issues.")
	} else {
		description.WriteString("A sub-service related to the application group is experiencing issues.")
	}
	description.WriteByte(' ')
	description.WriteString(o.getCurrentlyAffectedMessage())

	o.History = append(o.History, OutageHistoryFile{
		HistoryType: Investigating,
		Description: description.String(),
	})
	o.YamlContent.Severity = o.GetHighestSeverity()
}

func (o *Outage) RemoveHost(host OutageHost) {
	for i := range o.Hosts {
		if o.Hosts[i].Host == host.Host {
			o.Hosts = append(o.Hosts[:i], o.Hosts[i+1:]...)

			description := new(strings.Builder)
			description.WriteString("One of the services affected by the outage has re-established its services.")
			description.WriteByte(' ')
			description.WriteString(o.getCurrentlyAffectedMessage())
			o.History = append(o.History, OutageHistoryFile{
				HistoryType: Monitoring,
				Description: description.String(),
			})

			// If the host is not in the outage anymore, we need to update the status and resolve the outage
			if len(o.Hosts) == 0 {
				o.YamlContent.Resolved = true
				o.YamlContent.ResolvedWhen = time.Now().UTC().Format(time.RFC3339)
				o.History = append(o.History, OutageHistoryFile{
					HistoryType: Resolved,
					Description: "We can now confirm that all services related to the service are back online. The outage is now considered resolved.",
				})
			}

			break
		}
	}
	o.YamlContent.Severity = o.GetHighestSeverity()
}

func (o *Outage) GetHighestSeverity() string {
	highestSeverity := dto.Minor
	for index := range o.Hosts {
		if int(o.Hosts[index].Severity) > int(highestSeverity) {
			highestSeverity = o.Hosts[index].Severity
		}
	}

	switch highestSeverity {
	case dto.Critical:
		return "down"
	default:
		return "disrupted"
	}
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
	for index := len(o.History) - 1; index >= 0; index-- {
		historyContent.WriteString("**" + string(o.History[index].HistoryType) + "**" + " - " + o.History[index].Description + "\n\n")
	}

	// Format the outage file
	outageContent := new(strings.Builder)
	outageContent.WriteString("---\n")
	outageContent.Write(yamlContent)
	outageContent.WriteString("---\n")
	outageContent.WriteString(historyContent.String())

	return outageContent.String()
}
