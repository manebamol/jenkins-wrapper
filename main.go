package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	_ "io"
	_ "mime/multipart"
	"net/http"
	_ "os"
	"os/exec"
	"time"
)

// Jenkins credentials and details
const (
	jenkinsCLIPath = "" // Path to jenkins-cli.jar
	jenkinsURL     = "" // Jenkins URL
	jenkinsUser    = "" // Jenkins username
	jenkinsToken   = "" // Jenkins API token
	pluginName     = "" // Plugin name
	pluginPath     = "" // Path to the new plugin .hpi file
	jenkinsWarPath = "" // Path to jenkins.war
)

func isPluginInstalled() (bool, error) {
	url := fmt.Sprintf("%s/pluginManager/api/json?depth=1", jenkinsURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}
	req.SetBasicAuth(jenkinsUser, jenkinsToken)

	client := &http.Client{}
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

func uninstallPlugin() error {
	installed, err := isPluginInstalled()
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

	client := &http.Client{}
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

func installPlugin() error {
	cmd := exec.Command("java", "-jar", jenkinsCLIPath, "-s", jenkinsURL, "-auth", fmt.Sprintf("%s:%s", jenkinsUser, jenkinsToken), "install-plugin", fmt.Sprintf("file:///%s", pluginPath))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command execution failed: %v\nOutput: %s", err, output)
	}

	fmt.Println("✅ Plugin installed successfully!")
	fmt.Println(string(output))
	return nil
}

func stopJenkins() error {
	client := &http.Client{}
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

func startJenkins() error {
	cmd := exec.Command("cmd", "/C", "start", "java", "-jar", jenkinsWarPath)
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start Jenkins: %v", err)
	}
	fmt.Println("🚀 Jenkins started successfully.")
	return nil
}

func waitForJenkins() error {
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
	fmt.Println("🔄 Starting Jenkins plugin update process...")

	// Step 1: Uninstall the old plugin if it exists
	fmt.Println("🛑 Checking if plugin exists...")
	if err := uninstallPlugin(); err != nil {
		fmt.Println("Error:", err)
		return
	}

	time.Sleep(5 * time.Second)

	fmt.Println("⬆️ Uploading new plugin...")
	if err := installPlugin(); err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Step 2: Stop Jenkins using API
	fmt.Println("🛑 Stopping Jenkins...")
	if err := stopJenkins(); err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Wait for Jenkins to shut down completely
	time.Sleep(10 * time.Second)

	// Step 3: Start Jenkins
	fmt.Println("🚀 Starting Jenkins...")
	if err := startJenkins(); err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("🎉 Plugin update process completed successfully!")

	// Wait for Jenkins to restart
	if err := waitForJenkins(); err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Step 4: Check if the plugin is successfully installed
	time.Sleep(10 * time.Second)
	installed, err := isPluginInstalled()
	if err != nil {
		fmt.Println("Error checking installation:", err)
	} else if installed {
		fmt.Println("🎉 Plugin successfully installed!")
	} else {
		fmt.Println("❌ Plugin installation failed!")
	}
}
