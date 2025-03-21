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
)

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

	fmt.Println("✅ Plugin installed successfully!")
	fmt.Println(string(output))
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
	// Define command-line flags
	jenkinsCLIPath := flag.String("jenkinsCLIPath", "", "Path to jenkins-cli.jar")
	jenkinsURL := flag.String("jenkinsURL", "", "Jenkins URL")
	jenkinsUser := flag.String("jenkinsUser", "", "Jenkins user")
	jenkinsToken := flag.String("jenkinsToken", "", "Jenkins token")
	pluginName := flag.String("pluginName", "", "Plugin name")
	pluginPath := flag.String("pluginPath", "", "Path to plugin file")
	jenkinsWarPath := flag.String("jenkinsWarPath", "", "Path to jenkins.war")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s -jenkinsCLIPath=C:\\path\\to\\jenkins-cli.jar -jenkinsURL=http://localhost:8080 -jenkinsUser=admin -jenkinsToken=1234567890abcdef -pluginName=my-plugin -pluginPath=C:\\path\\to\\plugin.hpi -jenkinsWarPath=C:\\path\\to\\jenkins.war\n", os.Args[0])
	}

	// Parse command-line flags
	flag.Parse()

	// Check if required flags are provided
	if *jenkinsCLIPath == "" || *jenkinsURL == "" || *jenkinsUser == "" || *jenkinsToken == "" || *pluginName == "" || *pluginPath == "" || *jenkinsWarPath == "" {
		fmt.Println("All flags are required")
		flag.Usage()
		os.Exit(1)
	}

	if !isJenkinsRunning(*jenkinsURL) {
		fmt.Println("❌ Jenkins is not running. Please start Jenkins and try again.")
		os.Exit(1)
	}

	fmt.Println("🔄 Starting Jenkins plugin update process...")

	// Step 1: Uninstall the old plugin if it exists
	fmt.Println("🛑 Checking if plugin exists...")
	if err := uninstallPlugin(*jenkinsURL, *jenkinsUser, *jenkinsToken, *pluginName); err != nil {
		fmt.Println("Error:", err)
		return
	}

	time.Sleep(5 * time.Second)

	fmt.Println("⬆️ Uploading new plugin...")
	if err := installPlugin(*jenkinsCLIPath, *jenkinsURL, *jenkinsUser, *jenkinsToken, *pluginPath); err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Step 2: Stop Jenkins using API
	fmt.Println("🛑 Stopping Jenkins...")
	if err := stopJenkins(*jenkinsURL, *jenkinsUser, *jenkinsToken); err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Wait for Jenkins to shut down completely
	time.Sleep(10 * time.Second)

	// Step 3: Start Jenkins
	fmt.Println("🚀 Starting Jenkins...")
	if err := startJenkins(*jenkinsWarPath); err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("🎉 Plugin update process completed successfully!")

	// Wait for Jenkins to restart
	if err := waitForJenkins(*jenkinsURL); err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Step 4: Check if the plugin is successfully installed
	time.Sleep(10 * time.Second)
	installed, err := isPluginInstalled(*jenkinsURL, *jenkinsUser, *jenkinsToken, *pluginName)
	if err != nil {
		fmt.Println("Error checking installation:", err)
	} else if installed {
		fmt.Println("🎉 Plugin successfully installed!")
	} else {
		fmt.Println("❌ Plugin installation failed!")
	}
}
