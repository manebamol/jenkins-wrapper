package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/joho/godotenv"
)

// LoadEnv loads environment variables from a .env file
func LoadEnv() error {
	return godotenv.Load(".env")
}

// SaveEnv saves environment variables to a .env file
func SaveEnv(envMap map[string]string) error {
	file, err := os.OpenFile(".env", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	for key, value := range envMap {
		_, err := file.WriteString(fmt.Sprintf("%s=%s\n", key, value))
		if err != nil {
			return err
		}
	}
	return nil
}

func isJenkinsRunning(jenkinsURL string) bool {
	resp, err := http.Get(jenkinsURL + "/login")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func isPluginInstalled(jenkinsURL, jenkinsUser, jenkinsToken, pluginName string) (bool, error) {
	url := fmt.Sprintf("%s/pluginManager/api/json?depth=1", jenkinsURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}
	req.SetBasicAuth(jenkinsUser, jenkinsToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false, fmt.Errorf("failed to check plugin status: %s", resp.Status)
	}

	var result struct {
		Plugins []struct {
			ShortName string `json:"shortName"`
		} `json:"plugins"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	for _, plugin := range result.Plugins {
		if plugin.ShortName == pluginName {
			return true, nil
		}
	}

	return false, nil
}

func uninstallPlugin(jenkinsURL, jenkinsUser, jenkinsToken, pluginName string) error {
	installed, err := isPluginInstalled(jenkinsURL, jenkinsUser, jenkinsToken, pluginName)
	if err != nil {
		return err
	}
	if !installed {
		fmt.Println("⚠️ Plugin is not installed, skipping uninstallation.")
		return nil
	}

	url := fmt.Sprintf("%s/pluginManager/plugin/%s/doUninstall", jenkinsURL, pluginName)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(nil))
	if err != nil {
		return err
	}
	req.SetBasicAuth(jenkinsUser, jenkinsToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Println("✅ Plugin uninstalled successfully!")
	} else {
		return fmt.Errorf("failed to uninstall plugin: %s", resp.Status)
	}
	return nil
}

func installPlugin(jenkinsCLIPath, jenkinsURL, jenkinsUser, jenkinsToken, pluginPath string) error {
	cmd := exec.Command("java", "-jar", jenkinsCLIPath, "-s", jenkinsURL, "-auth", fmt.Sprintf("%s:%s", jenkinsUser, jenkinsToken), "install-plugin", fmt.Sprintf("file:///%s", pluginPath))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command execution failed: %v\nOutput: %s", err, output)
	}

	fmt.Println(string(output))
	fmt.Println("✅ Plugin installed successfully!")

	return nil
}

func stopJenkins(jenkinsURL, jenkinsUser, jenkinsToken string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/exit", jenkinsURL), nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(jenkinsUser, jenkinsToken)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Println("🛑 Jenkins is shutting down...")
	} else {
		fmt.Printf("❌ Failed to stop Jenkins: %s\n", resp.Status)
	}
	return nil
}

func startJenkins(jenkinsWarPath string) error {
	cmd := exec.Command("cmd", "/C", "start", "java", "-jar", jenkinsWarPath)
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start Jenkins: %v", err)
	}
	fmt.Println("🚀 Jenkins started successfully.")
	return nil
}

func waitForJenkins(jenkinsURL string) error {
	fmt.Println("⏳ Waiting for Jenkins to restart...")

	retries := 30 // Maximum wait time: 30 seconds
	for i := 0; i < retries; i++ {
		resp, err := http.Get(jenkinsURL + "/login")
		if err == nil && resp.StatusCode == 200 {
			fmt.Println("✅ Jenkins is back online!")
			return nil
		}
		fmt.Printf("🔄 Waiting... (%d/%d)\n", i+1, retries)
		time.Sleep(2 * time.Second) // Wait 2 seconds before retrying
	}
	return fmt.Errorf("jenkins did not restart in time")
}

func main() {
	// Load environment variables from .env file
	err := LoadEnv()
	if err != nil {
		fmt.Println("Error loading .env file:", err)
	}

	// Define command-line flags
	jenkinsCLIPath := flag.String("jenkinsCLIPath", os.Getenv("JENKINS_CLI_PATH"), "Path to jenkins-cli.jar")
	jenkinsURL := flag.String("jenkinsURL", os.Getenv("JENKINS_URL"), "Jenkins URL")
	jenkinsUser := flag.String("jenkinsUser", os.Getenv("JENKINS_USER"), "Jenkins user")
	jenkinsToken := flag.String("jenkinsToken", os.Getenv("JENKINS_TOKEN"), "Jenkins token")
	pluginName := flag.String("pluginName", os.Getenv("PLUGIN_NAME"), "Plugin name")
	pluginPath := flag.String("pluginPath", os.Getenv("PLUGIN_PATH"), "Path to plugin file")
	jenkinsWarPath := flag.String("jenkinsWarPath", os.Getenv("JENKINS_WAR_PATH"), "Path to jenkins.war")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s -jenkinsCLIPath=C:\\path\\to\\jenkins-cli.jar -jenkinsURL=http://localhost:8080 -jenkinsUser=admin -jenkinsToken=1234567890abcdef -pluginName=my-plugin -pluginPath=C:\\path\\to\\plugin.hpi -jenkinsWarPath=C:\\path\\to\\jenkins.war\n", os.Args[0])
	}

	// Parse command-line flags
	flag.Parse()

	// Load existing environment variables
	envMap := map[string]string{
		"JENKINS_CLI_PATH": os.Getenv("JENKINS_CLI_PATH"),
		"JENKINS_URL":      os.Getenv("JENKINS_URL"),
		"JENKINS_USER":     os.Getenv("JENKINS_USER"),
		"JENKINS_TOKEN":    os.Getenv("JENKINS_TOKEN"),
		"PLUGIN_NAME":      os.Getenv("PLUGIN_NAME"),
		"PLUGIN_PATH":      os.Getenv("PLUGIN_PATH"),
		"JENKINS_WAR_PATH": os.Getenv("JENKINS_WAR_PATH"),
	}

	// Update environment variables with provided flags
	if *jenkinsCLIPath != "" {
		envMap["JENKINS_CLI_PATH"] = *jenkinsCLIPath
	}
	if *jenkinsURL != "" {
		envMap["JENKINS_URL"] = *jenkinsURL
	}
	if *jenkinsUser != "" {
		envMap["JENKINS_USER"] = *jenkinsUser
	}
	if *jenkinsToken != "" {
		envMap["JENKINS_TOKEN"] = *jenkinsToken
	}
	if *pluginName != "" {
		envMap["PLUGIN_NAME"] = *pluginName
	}
	if *pluginPath != "" {
		envMap["PLUGIN_PATH"] = *pluginPath
	}
	if *jenkinsWarPath != "" {
		envMap["JENKINS_WAR_PATH"] = *jenkinsWarPath
	}

	// Save updated environment variables to .env file
	err = SaveEnv(envMap)
	if err != nil {
		fmt.Println("Error saving .env file:", err)
	}

	// Check if required environment variables are set
	if envMap["JENKINS_CLI_PATH"] == "" || envMap["JENKINS_URL"] == "" || envMap["JENKINS_USER"] == "" || envMap["JENKINS_TOKEN"] == "" || envMap["PLUGIN_NAME"] == "" || envMap["PLUGIN_PATH"] == "" || envMap["JENKINS_WAR_PATH"] == "" {
		fmt.Println("All flags are required")
		flag.Usage()
		os.Exit(1)
	}

	if !isJenkinsRunning(envMap["JENKINS_URL"]) {
		fmt.Println("❌ Jenkins is not running. Please start Jenkins and try again.")
		os.Exit(1)
	}

	fmt.Println("🔄 Starting Jenkins plugin update process...")

	// Step 1: Uninstall the old plugin if it exists
	fmt.Println("🛑 Checking if plugin exists...")
	if err := uninstallPlugin(envMap["JENKINS_URL"], envMap["JENKINS_USER"], envMap["JENKINS_TOKEN"], envMap["PLUGIN_NAME"]); err != nil {
		fmt.Println("Error:", err)
		return
	}

	time.Sleep(5 * time.Second)

	fmt.Println("⬆️ Uploading new plugin...")
	if err := installPlugin(envMap["JENKINS_CLI_PATH"], envMap["JENKINS_URL"], envMap["JENKINS_USER"], envMap["JENKINS_TOKEN"], envMap["PLUGIN_PATH"]); err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Step 2: Stop Jenkins using API
	fmt.Println("🛑 Stopping Jenkins...")
	if err := stopJenkins(envMap["JENKINS_URL"], envMap["JENKINS_USER"], envMap["JENKINS_TOKEN"]); err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Wait for Jenkins to shut down completely
	time.Sleep(10 * time.Second)

	// Step 3: Start Jenkins
	fmt.Println("🚀 Starting Jenkins...")
	if err := startJenkins(envMap["JENKINS_WAR_PATH"]); err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("🎉 Plugin update process completed successfully!")

	// Wait for Jenkins to restart
	if err := waitForJenkins(envMap["JENKINS_URL"]); err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Step 4: Check if the plugin is successfully installed
	time.Sleep(10 * time.Second)
	installed, err := isPluginInstalled(envMap["JENKINS_URL"], envMap["JENKINS_USER"], envMap["JENKINS_TOKEN"], envMap["PLUGIN_NAME"])
	if err != nil {
		fmt.Println("Error checking installation:", err)
	} else if installed {
		fmt.Println("🎉 Plugin successfully installed!")
	} else {
		fmt.Println("❌ Plugin installation failed!")
	}
}
