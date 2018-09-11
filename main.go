package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
	gin "gopkg.in/gin-gonic/gin.v1"
)

var (
	// The shared secret between the app and GitHub
	// This application only works with one shared secret. It was designed for
	// a single repo (and single repo situation). The app should be modified
	// for a multi-repo context where each repo can have its own secret
	sharedSecret string

	// The name of the repo e.g. "foo/bar"
	repoFullName string

	// The token for the user/bott that will be updating the label and sending
	// a notification
	ghToken string

	testRe = regexp.MustCompile(`(?mi)^Signed-off-by: (.*) <(.*)>$`)
)

const (
	statePending = "pending"
	stateFailure = "failure"
	stateSuccess = "success"
)

// TODO(mattfarina): rewrite to be more betterer

func main() {

	// Get config from environment
	sharedSecret = os.Getenv("GITHUB_SHARED_SECRET")
	repoFullName = os.Getenv("GITHUB_REPO_NAME")
	ghToken = os.Getenv("GITHUB_TOKEN")

	// Disable color in output
	gin.DisableConsoleColor()

	router := gin.Default()

	// We can use this to check the health or and make sure the app is online
	router.GET("/healthz", healthz)

	router.POST("/webhook", processHook)

	router.Run()
}

func healthz(c *gin.Context) {
	c.String(http.StatusOK, http.StatusText(http.StatusOK))
}

func processHook(c *gin.Context) {

	// Validate payload
	sig := c.GetHeader("X-Hub-Signature")
	if sig == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Missing X-Hub-Signature"})
		return
	}

	body, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		logit("ERROR: Failed to read request body: %s", err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Malformed body"})
		return
	}
	defer c.Request.Body.Close()

	if err := validateSig(sig, body); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"message": "Validating payload against signature failed"})
		return
	}

	// Check the event type. Make sure just a PR
	// We need to get the event from the Request object as Gin in the middle
	// does some normalization that breaks this particular header name.
	event := c.Request.Header.Get("X-GitHub-Event")
	if event != "pull_request" {
		c.JSON(http.StatusOK, gin.H{"message": "Skipping event type"})
		return
	}

	// Get the payload body as an object
	e, err := github.ParseWebHook(event, body)
	if err != nil {
		logit("ERROR: Failed to parse body: %s", err)
		c.JSON(http.StatusBadRequest, gin.H{"message": "Malformed body"})
		return
	}
	payload := e.(*github.PullRequestEvent)

	// Check this is a repo we operate on
	if *payload.Repo.FullName != repoFullName {
		c.JSON(http.StatusForbidden, gin.H{"message": "Not configured for this repository"})
		return
	}

	// Filter pull request actions we aren't intersted in like labels being added/removed
	if *payload.Action != "opened" && *payload.Action != "synchronize" && *payload.Action != "reopened" {
		c.JSON(http.StatusOK, gin.H{"message": "Skipping action"})
		return
	}

	// Send initial notification
	err = sendNotification(payload.PullRequest.Head.GetSHA(), statePending, "Checking for DCO")
	if err != nil {

		// Not exiting on this error because technically we can continue but we
		// are likely to run into future problems.
		logit("ERROR: Unable to sending checking notification for PR %s: %s", *payload.Number, err)
	}

	// Get commits
	commits, err := getCommits(*payload.Number)
	if err != nil {
		logit("ERROR: Unable to get PR commits for PR %s: %s", *payload.Number, err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Error processing request (1)"})
		return
	}

	// Check commits for DCO
	count := 0
	missed := 0
	for _, commit := range commits {
		count++
		message := commit.GetCommit().GetMessage()
		if !testRe.MatchString(message) {
			missed++
		}
	}

	// If they have DCO 1. add label and 2. send notification
	if missed == 0 {

		// Add label
		labels := []string{"Contribution Allowed"}
		err = addLabel(*payload.Number, labels)
		if err != nil {
			logit("ERROR: Unable to add label to PR %s: %s", *payload.Number, err)
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Error processing request (2)"})
			return
		}

		// Send Notification
		err = sendNotification(payload.PullRequest.Head.GetSHA(), stateSuccess, "All commits have signoff")
		if err != nil {

			logit("ERROR: Unable to sending success notification for PR %s: %s", *payload.Number, err)
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Error processing request (3)"})
			return
		}
	} else {
		// If no DCO 1. Remove label and 2. send notification
		err = removeLabel(*payload.Number, "Contribution Allowed")
		if err != nil {

			logit("ERROR: Unable to remove label from PR %s: %s", *payload.Number, err)
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Error processing request (4)"})
			return
		}

		// Send Notification
		msg := fmt.Sprintf("%d out of %d commits are missing signoff", missed, count)
		err = sendNotification(payload.PullRequest.Head.GetSHA(), stateFailure, msg)
		if err != nil {

			logit("ERROR: Unable to sending failure notification for PR %s: %s", *payload.Number, err)
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Error processing request (5)"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Success"})
}

func validateSig(sig string, body []byte) error {
	digest := hmac.New(sha1.New, []byte(sharedSecret))
	digest.Write(body)
	sum := digest.Sum(nil)
	checksum := fmt.Sprintf("sha1=%x", sum)
	if subtle.ConstantTimeCompare([]byte(checksum), []byte(sig)) != 1 {
		logit("ERROR: Expected signature %q, but got %q", checksum, sig)
		return errors.New("payload signature check failed")
	}
	return nil
}

func sendNotification(commit, state, desc string) error {
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("expected repository for form org/repo, got %q", repoFullName)
	}

	c, client := ghClient()

	ctx := "dco-labeler"

	status := &github.RepoStatus{
		State:       &state,
		Description: &desc,
		Context:     &ctx,
	}

	_, _, err := client.Repositories.CreateStatus(
		c,
		parts[0],
		parts[1],
		commit,
		status,
	)

	return err
}

func getCommits(num int) ([]*github.RepositoryCommit, error) {
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("expected repository for form org/repo, got %q", repoFullName)
	}

	c, client := ghClient()

	opts := &github.ListOptions{
		Page: 0,

		// 100 is the API max. If we ever end up with more than 100 we will
		// need to add pagenation
		PerPage: 100,
	}
	commits, _, err := client.PullRequests.ListCommits(c, parts[0], parts[1], num, opts)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch commits: %s", err)
	}

	return commits, nil
}

func addLabel(num int, labels []string) error {
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("expected repository for form org/repo, got %q", repoFullName)
	}

	c, client := ghClient()

	_, _, err := client.Issues.AddLabelsToIssue(c, parts[0], parts[1], num, labels)

	return err
}

func removeLabel(num int, label string) error {
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("expected repository for form org/repo, got %q", repoFullName)
	}

	c, client := ghClient()

	res, err := client.Issues.RemoveLabelForIssue(c, parts[0], parts[1], num, label)

	if res.StatusCode == 404 {
		return nil
	}

	return err
}

func logit(message string, vars ...interface{}) {
	fmt.Fprintf(gin.DefaultWriter, "[APP] "+message+"\n", vars...)
}

func ghClient() (context.Context, *github.Client) {
	t := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: ghToken})
	c := context.Background()
	tc := oauth2.NewClient(c, t)
	return c, github.NewClient(tc)
}
