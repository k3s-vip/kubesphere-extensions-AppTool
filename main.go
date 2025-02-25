package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/phuslu/log"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/yaml"
)

type HelmIndex struct {
	Entries map[string][]struct {
		Name    string   `json:"name"`
		Version string   `json:"version"`
		URLs    []string `json:"urls"`
	} `json:"entries"`
}

type AppRequest struct {
	RepoName     string `json:"repoName"`
	Package      string `json:"package"`
	CategoryName string `json:"categoryName"`
	Workspace    string `json:"workspace"`
	AppType      string `json:"appType"`
}

var (
	versionGVR = schema.GroupVersionResource{
		Group:    "application.kubesphere.io",
		Version:  "v2",
		Resource: "applicationversions",
	}
	appGVR = schema.GroupVersionResource{
		Group:    "application.kubesphere.io",
		Version:  "v2",
		Resource: "applications",
	}
	mark          = "openpitrix-import"
	dynamicClient *dynamic.DynamicClient
	serverURL     string
	token         string
	repoURL       string
	latestZ       bool
	latestMax     int
	username      string
	password      string
)

func init() {
	log.DefaultLogger = log.Logger{
		TimeFormat: "15:04:05",
		Caller:     1,
		Writer: &log.ConsoleWriter{
			ColorOutput:    true,
			QuoteString:    true,
			EndWithMessage: true,
		},
	}
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "AppTool for KubeSphere",
		Short: "A CLI tool to manage applications",
		Run: func(cmd *cobra.Command, args []string) {
			run()
		},
	}

	rootCmd.Flags().StringVarP(&serverURL, "server", "s", "http://10.0.1.127:30880", "Kubesphere Server URL")
	rootCmd.Flags().StringVarP(&repoURL, "repo", "r", "https://charts.longhorn.io", "Helm index URL")
	rootCmd.Flags().StringVarP(&token, "token", "t", "", "token")
	rootCmd.Flags().BoolVarP(&latestZ, "LatestZ", "Z", true, "ChartVerison LatestZ")
	rootCmd.Flags().IntVarP(&latestMax, "latestMax", "M", 3, "ChartVerison max LatestXY")
	rootCmd.Flags().StringVarP(&username, "username", "u", "admin", "Username")
	rootCmd.Flags().StringVarP(&password, "password", "p", "P@88w0rd", "Password")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() {
	log.Info().Msgf("Starting to upload to %s ", serverURL)

	err := initDynamicClient()
	if err != nil {
		log.Fatal().Msgf("Failed to initialize dynamic client: %v", err)
	}

	err = uploadChart()
	if err != nil {
		log.Fatal().Msgf("Failed to upload chart: %v", err)
	}

	listOptions := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("application.kubesphere.io/app-category-name=%s", mark),
	}

	err = updateAppStatus(listOptions)
	if err != nil {
		log.Fatal().Msgf("[1/4] Failed to update app status: %v", err)
	}
	log.Info().Msgf("[1/4] updateAppStatus completed successfully")

	store := map[string]string{"application.kubesphere.io/app-store": "true"}
	err = updateAppLabel(listOptions, store)
	if err != nil {
		log.Fatal().Msgf("[2/4] Failed to update app label: %v", err)
	}
	log.Info().Msgf("[2/4] updateAppLabel store completed successfully")

	err = updateVersionStatus(listOptions)
	if err != nil {
		log.Fatal().Msgf("[3/4] Failed to update version status: %v", err)
	}
	log.Info().Msgf("[3/4] updateVersionStatus completed successfully")

	categoryName := map[string]string{"application.kubesphere.io/app-category-name": "kubesphere-app-uncategorized"}
	err = updateAppLabel(listOptions, categoryName)
	if err != nil {
		log.Fatal().Msgf("[4/4] Failed to update app category label: %v", err)
	}
	log.Info().Msgf("[4/4] updateAppLabel categoryName completed successfully")
}

func initDynamicClient() (err error) {
	conf := config.GetConfigOrDie()
	dynamicClient, err = dynamic.NewForConfig(conf)
	if err != nil {
		log.Error().Msgf("Failed to create dynamic client: %v", err)
		return err
	}
	log.Info().Msgf("Dynamic client initialized successfully")
	return nil
}

func uploadChart() error {
	u := fmt.Sprintf("%s/index.yaml", repoURL)
	indexData, err := fetchIndex(u)
	if err != nil {
		log.Error().Msgf("Failed to fetch Helm index: %v", err)
		return err
	}

	for _, entries := range indexData.Entries {
		var appID string
		appLastXY, appLatestZ, appLatest, appLatestMax := "", 0, 0, latestMax
		appVerSkip := make(map[string]int)
		for idx, entry := range entries {
			appVer := entry.Version
			appVersion := strings.Split(appVer, ".")
			appVerZ, _ := strconv.Atoi(appVersion[len(appVersion)-1])
			if !strings.Contains(appVer, appLastXY) {
				appLatestZ = 0
			}
			if appVerZ > appLatestZ {
				appLatestZ = appVerZ
			} else {
				appVerSkip[entry.Version] = idx
			}
			appLastXY = strings.Join(appVersion[:len(appVersion)-1], ".")
		}
		for idx, entry := range entries {
			if _, ok := appVerSkip[entry.Version]; ok && latestZ {
				log.Debug().Msgf("Skip%-3d:%s", idx, entry.Version)
				continue
			}
			appLatest++
			if appLatest > appLatestMax {
				break
			}
			chartURL := entry.URLs[0]
			if strings.Contains(chartURL, "https://github.com/") {
				chartURL = strings.Replace(chartURL, "github.com", "ghfast.top/github.com", -1)
			}
			chartData, err := fetchChart(chartURL)
			if err != nil {
				log.Error().Msgf("Failed to fetch chart %s: %v", entry.Name, err)
				continue
			}

			appRequest := AppRequest{
				RepoName:     "upload",
				Package:      base64.StdEncoding.EncodeToString(chartData),
				CategoryName: mark,
				Workspace:    "",
				AppType:      "helm",
			}

			var url string
			if idx == 0 {
				url = fmt.Sprintf("%s/kapis/application.kubesphere.io/v2/apps", serverURL)
				appID, err = upload(appRequest, entry.Name, entry.Version, url)
				if err != nil {
					log.Error().Msgf("Failed to post app %s: %v", entry.Name, err)
					appID = "" // Reset appID to empty string on failure
					continue
				}
			} else {
				if appID == "" {
					log.Error().Msgf("Skipping version %s for app %s due to missing appID", entry.Version, entry.Name)
					continue
				}
				url = fmt.Sprintf("%s/kapis/application.kubesphere.io/v2/apps/%s/versions", serverURL, appID)
				_, err = upload(appRequest, entry.Name, entry.Version, url)
				if err != nil {
					log.Error().Msgf("Failed to post app version %s:%s %v", entry.Name, entry.Version, err)
					continue
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	return nil
}

func fetchIndex(url string) (*HelmIndex, error) {
	resp, err := http.Get(url)
	if err != nil {
		log.Error().Msgf("Failed to fetch index: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error().Msgf("Failed to read response body: %v", err)
		return nil, err
	}

	var index HelmIndex
	err = yaml.Unmarshal(body, &index)
	if err != nil {
		log.Error().Msgf("Failed to unmarshal index: %v", err)
		return nil, err
	}

	return &index, nil
}

func fetchChart(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		log.Error().Msgf("Failed to fetch chart: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error().Msgf("Failed to read response body: %v", err)
		return nil, err
	}
	return body, nil
}

func upload(appRequest AppRequest, name, version, url string) (appID string, err error) {
	jsonData, _ := json.Marshal(appRequest)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Error().Msgf("Failed to create request: %v", err)
		return "", err
	}

	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	} else {
		if token == "" {
			dst := "/run/secrets/kubernetes.io/serviceaccount/token"
			data, err := os.ReadFile(dst)
			log.Warn().Msgf("try read %s", dst)
			if err != nil {
				log.Error().Msgf("%v", err)
			}
			token = string(data)
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Msgf("Failed to send request: %v", err)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		log.Fatal().Msgf("Failed to find app store manager, please check if it is installed")
		return "", fmt.Errorf("please check if app store manager is installed")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to post app, status code: %d", resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error().Msgf("Failed to read response body: %v", err)
		return "", err
	}
	var response struct {
		AppName string `json:"appName"`
	}
	err = json.Unmarshal(body, &response)
	if err != nil {
		log.Error().Msgf("Failed to unmarshal response body:%s,  %v", string(body), err)
		return "", err
	}

	log.Info().Msgf("App %s:%s posted successfully", name, version)
	return response.AppName, nil
}

func updateVersionStatus(listOptions metav1.ListOptions) error {
	list, err := dynamicClient.Resource(appGVR).List(context.TODO(), listOptions)
	if err != nil {
		log.Error().Msgf("Failed to list apps: %v", err)
		return err
	}

	for _, item := range list.Items {
		options := metav1.ListOptions{
			LabelSelector: fmt.Sprintf("application.kubesphere.io/app-id=%s", item.GetName()),
		}
		versionList, err := dynamicClient.Resource(versionGVR).List(context.TODO(), options)
		if err != nil {
			log.Error().Msgf("Failed to list versions for app %s: %v", item.GetName(), err)
			return err
		}

		for _, versionItem := range versionList.Items {
			currentTime := time.Now().UTC().Format(time.RFC3339)
			unstructured.SetNestedField(versionItem.Object, currentTime, "status", "updated")
			unstructured.SetNestedField(versionItem.Object, "admin", "status", "userName")
			unstructured.SetNestedField(versionItem.Object, "active", "status", "state")

			_, err := dynamicClient.Resource(versionGVR).UpdateStatus(context.TODO(), &versionItem, metav1.UpdateOptions{})
			if err != nil {
				log.Error().Msgf("Failed to update version status for app %s: %v", item.GetName(), err)
				return err
			}
		}
	}

	return nil
}

func updateAppLabel(listOptions metav1.ListOptions, label map[string]string) error {
	list, err := dynamicClient.Resource(appGVR).List(context.TODO(), listOptions)
	if err != nil {
		log.Error().Msgf("Failed to list apps: %v", err)
		return err
	}

	for _, item := range list.Items {
		labels := item.GetLabels()
		for k, v := range label {
			labels[k] = v
		}

		item.SetLabels(labels)
		_, err = dynamicClient.Resource(appGVR).Update(context.TODO(), &item, metav1.UpdateOptions{})
		if err != nil {
			log.Error().Msgf("Failed to update labels for app %s: %v", item.GetName(), err)
			return err
		}
	}

	return nil
}

func updateAppStatus(listOptions metav1.ListOptions) error {
	list, err := dynamicClient.Resource(appGVR).List(context.TODO(), listOptions)
	if err != nil {
		log.Error().Msgf("Failed to list apps: %v", err)
		return err
	}

	for _, item := range list.Items {
		currentTime := time.Now().UTC().Format(time.RFC3339)
		unstructured.SetNestedField(item.Object, "active", "status", "state")
		unstructured.SetNestedField(item.Object, currentTime, "status", "updateTime")

		_, err := dynamicClient.Resource(appGVR).UpdateStatus(context.TODO(), &item, metav1.UpdateOptions{})
		if err != nil {
			log.Error().Msgf("Failed to update status for app %s: %v", item.GetName(), err)
			return err
		}
	}

	return nil
}
