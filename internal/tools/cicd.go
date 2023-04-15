package tools

import (
	"encoding/json"
	"log"
	"os"
	"os/user"
	"strconv"
	"strings"

	git "github.com/go-git/go-git/v5"
)

func getGithubPRNumber(eventFileName string) string {
	// Read the JSON file
	jsonData, err := os.ReadFile(eventFileName)
	if err != nil {
		panic(err)
	}

	// Unmarshal the JSON data into a map
	var data map[string]interface{}
	err = json.Unmarshal(jsonData, &data)
	if err != nil {
		panic(err)
	}

	// Print the data
	return strconv.Itoa(int(data["number"].(float64)))
}

// GenerateMachineUniqueName generates a unique name for the machine based on
// the CI environment.
func GenerateMachineUniqueName() string {
	var name string
	if os.Getenv("CI") == "true" && os.Getenv("GITHUB_ACTIONS") == "true" {
		repo := strings.Replace(os.Getenv("GITHUB_REPOSITORY"), "/", "-", -1)
		name = repo + "-PR" + getGithubPRNumber(os.Getenv("GITHUB_EVENT_PATH"))
	} else if os.Getenv("CI") == "true" && os.Getenv("CIRCLECI") == "true" {
		name = os.Getenv("CIRCLE_PROJECT_REPONAME") + "-PR" + os.Getenv("CIRCLE_PR_NUMBER")
	} else if os.Getenv("CI") == "true" && os.Getenv("GITLAB_CI") == "true" {
		name = os.Getenv("CI_PROJECT_PATH") + "-PR" + os.Getenv("CI_MERGE_REQUEST_IID")
	} else if os.Getenv("CI") == "true" && os.Getenv("BITBUCKET_COMMIT") != "" {
		name = os.Getenv("BITBUCKET_REPO_SLUG") + "-PR" + os.Getenv("BITBUCKET_PR_ID")
	} else if os.Getenv("CI") == "true" && os.Getenv("TRAVIS") == "true" {
		name = os.Getenv("TRAVIS_REPO_SLUG") + "-PR" + os.Getenv("TRAVIS_PULL_REQUEST")
	} else {
		log.Println("[DEBUG] Not running in CI, checking if you're on a git repository")
		if _, err := os.Stat(".git"); !os.IsNotExist(err) {
			log.Println("[DEBUG] .git directory found, using git branch name as instance name")
			r, err := git.PlainOpen(".")
			if err != nil {
				log.Fatal(err)
			}
			ref, err := r.Head()
			if err != nil {
				log.Fatal(err)
			}
			name = ref.Name().Short()
		} else {
			// If not in CI and not in a git repository, use the current user name
			log.Println("[DEBUG] .git directory not found, using current user name and directory name")
			dir, err := os.Getwd()
			log.Println("[DEBUG] current dir: " + dir)
			if err != nil {
				log.Fatal(err)
			}
			name = strings.Replace(dir[strings.LastIndex(dir, "/")+1:], "/", "-", -1)
		}
		currentUser, err := user.Current()
		if err != nil {
			log.Fatalf(err.Error())
		}

		return currentUser.Username + "-" + name
	}
	log.Println("[DEBUG] instanceName: " + name)
	return name
}
