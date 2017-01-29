package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	. "github.com/CodyGuo/win"
	"github.com/hashicorp/go-version"
	"github.com/kardianos/osext"
	"github.com/yhat/scrape"
	"golang.org/x/net/html"
	"golang.org/x/sys/windows"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

var envRegex = regexp.MustCompile(`%[^%]+%`)

const choco = "C:/ProgramData/chocolatey/bin/choco.exe"
const ps5url = "https://www.microsoft.com/en-us/download/confirmation.aspx?id=50395"
const dotNetVersion = "4.6"
const chocoRebootCode = 3010

var tempdir string

type OS struct {
	Version OSVersion
	Type    OSType
}

type OSType string
type OSVersion string

const (
	OSTypeUndefined OSType = ""
	X86                    = "x86"
	X64                    = "x64"
)

const (
	OSVersionUndefined OSVersion = ""
	Win10                        = "Windows 10"
	Win7                         = "Windows 7"
	WinXP                        = "Windows XP"
)

var OSKeywords = map[OSVersion][]string{
	Win10: []string{"win10", "windows 10"},
	Win7:  []string{"win7", "windows 7"},
	WinXP: []string{},
}

var OSTypeKeywords = map[OSType][]string{
	X86: []string{"x86", "i386"},
	X64: []string{"x64", "amd64"},
}

var ThisOS = OS{OSVersionUndefined, OSTypeUndefined}

func CheckError(err error) {
	if err != nil {
		// fmt.Println("Error: ", err)
		fmt.Fprintf(os.Stderr, "Error: : %s", err)
	}
}

func CheckErrorFatal(err error) {
	if err != nil {
		CheckError(err)
		reader := bufio.NewReader(os.Stdin)
		reader.ReadString('\n')
		log.Panic(err)
	}
}

func downloadFile(filepath string, url string) (err error) {

	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Writer the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func setTempDir() {
	if runtime.GOOS == "windows" {
		tempdir = os.Getenv("TMP") + "\\"
	} else {
		tempdir = "/tmp/"
	}

}

func installChoco() {
	// check if choco is already installed
	fmt.Println("Checking if choco is installed")
	_, err := exec.Command(choco, "search", "dotnet4.6").Output()
	if err != nil {
		fmt.Println("Choco not found:", err)
		fmt.Println("Setting powershell execution policy to 'Bypass'")
		cmd := exec.Command("powershell", "Set-ExecutionPolicy", "Bypass")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Start()
		CheckErrorFatal(err)
		err = cmd.Wait()
		CheckErrorFatal(err)
		fmt.Println("Installing choco...")
		cmd = exec.Command("powershell", "iex ((new-object net.webclient).DownloadString('https://chocolatey.org/install.ps1'))")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Start()
		CheckErrorFatal(err)
		err = cmd.Wait()
		CheckErrorFatal(err)
	} else {
		fmt.Println("Choco is already installed")
	}
}

func installDotNet() {
	fmt.Println("Checking if dotnet4.6 is installed")
	f := "dotNetVersion.exe"
	path := tempdir + f
	extractFile(f, path)
	shouldInstall := false
	out, err := exec.Command(path, "nopause").Output()
	if err != nil {
		fmt.Println("Cannot check dotnet version: ", err)
		shouldInstall = true
	} else {
		fmt.Printf("Detected dotnet version: '%s'\n", string(out))
		current, err := version.NewVersion(string(out))
		CheckErrorFatal(err)
		required, err := version.NewVersion(dotNetVersion)
		CheckErrorFatal(err)
		if current.LessThan(required) {
			fmt.Printf("%s is less than %s\n", current, required)
			shouldInstall = true
		}
	}

	removeFileIfExist(path)
	if shouldInstall {
		ChocoInstall("dotnet" + dotNetVersion)
	}
}

func ChocoInstall(name string) {
	fmt.Printf("Choco installing or upgrading %s...\n", name)
	cmd := exec.Command(choco, "upgrade", "-y", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	CheckErrorFatal(err)
	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				exitCode := status.ExitStatus()
				fmt.Printf("Exit Status: %d\n", exitCode)
				if exitCode == chocoRebootCode {
					createRebootFlag()
				} else {
					CheckErrorFatal(err)
				}
			} else {
				CheckErrorFatal(errors.New("Status error"))
			}
		} else {
			CheckErrorFatal(errors.New(fmt.Sprintf("cmd.Wait: %v\n", err)))
		}
	}

}

func GetPS5URLs() map[string]map[string]string {
	response, err := http.Get(ps5url)
	CheckErrorFatal(err)

	defer response.Body.Close()

	data, _ := ioutil.ReadAll(response.Body)

	// remove noscript tags, https://github.com/golang/go/issues/16318
	reg, err := regexp.Compile("</?noscript>")
	CheckErrorFatal(err)

	safeHTMLBytes := reg.ReplaceAll(data, []byte{})

	root, err := html.Parse(bytes.NewBuffer(safeHTMLBytes))
	CheckErrorFatal(err)

	var f func(*html.Node) *html.Node
	f = func(n *html.Node) *html.Node {
		if n.Type == html.ElementNode {
			for _, c := range n.Attr {
				if c.Val == "chooseFile jsOff" {
					return n
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			cn := f(c)
			if cn != nil {
				return cn
			}
		}
		return nil
	}
	div := f(root)

	fileSpans := scrape.FindAll(div, scrape.ByClass("file-name"))

	downloads := make(map[string]map[string]string)

	for _, fileSpan := range fileSpans {
		fileName, fileNameOK := scrape.Find(fileSpan, scrape.ByClass("file-name-view1"))
		fileSize, fileSizeOK := scrape.Find(fileSpan, scrape.ByClass("file-size-view1"))
		fileURL, fileURLOK := scrape.Find(fileSpan, scrape.ByClass("file-link-view1"))
		if fileNameOK && fileSizeOK && fileURLOK {
			// fmt.Printf("%+v %+v %+v;\n", scrape.Attr(fileURL.FirstChild, "href"), scrape.Text(fileName), scrape.Text(fileSize))
			downloads[scrape.Text(fileName)] = map[string]string{"url": scrape.Attr(fileURL.FirstChild, "href"), "size": scrape.Text(fileSize)}
		}
	}
	return downloads
}

func GetPS5URLForOS(os OS) (map[string]string, error) {
	mapURLs := GetPS5URLs()
	for k, _ := range mapURLs {
		if checkOSCompatibilityBySoftwareName(k, ThisOS) {
			return mapURLs[k], nil
		}
	}
	return nil, errors.New(fmt.Sprintf("Didn't find ps5 installer for %s", os.Type))
}

func installPS5() {
	fmt.Println("Checking powershell version if any")
	out, err := exec.Command("powershell", "$PSVersionTable.PSVersion.Major").Output()
	if err != nil {
		fmt.Println("Cannot determine powershell version: ", err)
	} else {
		rawPSVersion := strings.TrimSpace(string(out))
		v, err := strconv.Atoi(rawPSVersion)
		if err != nil {
			fmt.Println("Cannot parse powershell version: ", rawPSVersion, " to integer: ", err)
		} else {
			fmt.Println("powershell version: ", v)
			if v < 5 {
				fmt.Printf("powershell version %d is lower then required 5, going to upgrade\n", v)
				fmt.Printf("Checking for avaialbe versions for %s %s on %s\n", ThisOS.Version, ThisOS.Type, ps5url)

				ps5DownloadDetails, err := GetPS5URLForOS(ThisOS)
				CheckErrorFatal(err)
				url := ps5DownloadDetails["url"]

				uris := strings.Split(url, "/")
				pkg := uris[len(uris)-1]
				path := tempdir + pkg
				fmt.Printf("Downloading %s to %s\n", pkg, path)
				downloadFile(path, url)
				out, err := exec.Command("cmd", "/c "+path+" /quiet").Output() // try /q /norestart for dotnet
				fmt.Println("out: ", string(out))
				CheckErrorFatal(err)
				removeFileIfExist(path)
			}
		}
	}
}

func checkOSCompatibilityBySoftwareName(name string, os OS) bool {
	for _, osKeyWord := range OSKeywords[os.Version] {
		matched, err := regexp.MatchString("(?i)"+osKeyWord, name)
		CheckErrorFatal(err)
		if matched {
			for _, osTypeKeyWord := range OSTypeKeywords[os.Type] {
				matched, err := regexp.MatchString("(?i)"+osTypeKeyWord, name)
				CheckErrorFatal(err)
				if matched {
					return true
				}
			}
		}
	}
	return false
}

func removeFileIfExist(path string) {
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("Removing %s... ", path)
		if err := os.Remove(path); err != nil {
			fmt.Println("Failed")
		} else {
			fmt.Println("Done")
		}
	}
}

func removeFileIfExistOnReboot(path string) {
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("Marking %s for removal on reboot\n", path)
		from, err := syscall.UTF16PtrFromString(path)
		CheckErrorFatal(err)

		windows.MoveFileEx(from, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
	}
}

func extractFile(f string, path string) (err error) {
	data, err := Asset(f)
	if err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	if err != nil {
		return err
	}
	file.Sync()
	return nil
}

func configForAnsible() {
	f := "ConfigureRemotingForAnsible.ps1"
	path := tempdir + f
	extractFile(f, path)

	fmt.Println("Setting powershell execution policy to 'Bypass'")
	cmd := exec.Command("powershell", "Set-ExecutionPolicy", "Bypass")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	CheckErrorFatal(err)
	err = cmd.Wait()
	CheckErrorFatal(err)

	fmt.Println("Preparing host for ansible...")
	changeTypeForPublicNetworks()
	cmd = exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	CheckErrorFatal(err)
	err = cmd.Wait()
	CheckErrorFatal(err)

	removeFileIfExist(path)
}

func getWindowsStartupPath() string {
	fmt.Println("Getting windows startup path:")
	// out, err := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-NoLogo", "-NonInteractive", "-NoProfile", "-Command", "[environment]::getfolderpath('Startup')").Output()
	out, err := exec.Command("powershell", "-Command", "[environment]::getfolderpath('Startup')").Output()
	startupPath := strings.TrimSpace(string(out))
	fmt.Println(startupPath)
	CheckErrorFatal(err)
	return startupPath
}

func addToStartup() {
	startupPath := getWindowsStartupPath()

	fmt.Println("Getting current file path:")
	filePath, err := osext.Executable()
	fmt.Println(filePath)
	CheckErrorFatal(err)

	uris := strings.Split(filePath, "\\")
	exe := uris[len(uris)-1]

	if filePath != tempdir+exe {
		fmt.Printf("Copy file from: %s\nto %%TEMP%% path: %s\n", filePath, tempdir+exe)
		_, err = exec.Command("xcopy", "/Y", filePath, tempdir).Output()
		CheckErrorFatal(err)
	} else {
		fmt.Println("Already started %%TEMP%% startup")
	}
	fmt.Println("Adding cmd to startup path")
	// err := ioutil.WriteFile(", ), 0644)

	file, err := os.Create(startupPath + "\\startGoansible.cmd")
	CheckErrorFatal(err)
	defer file.Close()
	_, err = file.Write([]byte("start \"\" " + tempdir + exe))
	CheckErrorFatal(err)
	file.Sync()
}

func removeFromStartup() {
	startupPath := getWindowsStartupPath()
	filePath, err := osext.Executable()
	CheckErrorFatal(err)
	uris := strings.Split(filePath, "\\")
	exe := uris[len(uris)-1]
	removeFileIfExistOnReboot(tempdir + "\\" + exe)
	removeFileIfExist(startupPath + "\\startGoansible.cmd")
}

func createRebootFlag() {
	path := tempdir + "goansibleRebootFlag"
	fmt.Printf("Creating reboot flag at %s\n", path)
	flagFile, err := os.Create(path)
	CheckErrorFatal(err)
	flagFile.Close()
	removeFileIfExistOnReboot(path)
}

func removeRebootFlag() {
	removeFileIfExistOnReboot(tempdir + "goansibleRebootFlag")
}

func rebootIfRequired() {
	path := tempdir + "goansibleRebootFlag"
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("Reboot is required...")
		reboot()
	} else {
		fmt.Printf("No reboot flag found at %s\n", path)
	}
}

func changeTypeForPublicNetworks() {
	f := "ChangeCategory.ps1"
	path := tempdir + f
	extractFile(f, path)

	fmt.Println("Setting powershell execution policy to 'Bypass'")
	out, err := exec.Command("powershell", "Set-ExecutionPolicy", "Bypass").Output()
	if err != nil {
		fmt.Println("Cannot set powershell execution policy to 'Bypass': ", err)
	} else {
		fmt.Println(string(out))
		fmt.Println("Checking for public networks, they will prevent wmic from starting")
		out, err := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", path).Output()
		if err != nil {
			fmt.Println("Stdout: ", string(out))
			CheckErrorFatal(err)
		} else {
			fmt.Println(string(out))
		}
	}
	removeFileIfExist(path)
}

func getSystemInfo() {
	out, err := exec.Command("systeminfo", "/FO", "CSV", "/NH").Output()
	if err != nil {
		fmt.Println("Can't get system info:", err)
	} else {
		// fmt.Println(string(out))
		r := csv.NewReader(strings.NewReader(string(out)))
		record, err := r.Read()
		CheckErrorFatal(err)
		osName := record[1]
		osType := record[13]
		switch {
		case strings.Contains(osType, "X86"):
			ThisOS.Type = X86
		case strings.Contains(osType, "x64"):
			ThisOS.Type = X64
		default:
			CheckErrorFatal(errors.New(fmt.Sprintf("Unknown OS type: %s", osType)))
		}
		switch {
		case strings.Contains(osName, "Windows 7 "):
			ThisOS.Version = Win7
		case strings.Contains(osName, "Windows 10 "):
			ThisOS.Version = Win10
		case strings.Contains(osName, "Windows XP "):
			ThisOS.Version = WinXP
		default:
			CheckErrorFatal(errors.New(fmt.Sprintf("Unknown OS type: %s", osType)))
		}
	}
	fmt.Printf("%+v\n", ThisOS)
}

func getPrivileges() {
	var hToken HANDLE
	var tkp TOKEN_PRIVILEGES

	OpenProcessToken(GetCurrentProcess(), TOKEN_ADJUST_PRIVILEGES|TOKEN_QUERY, &hToken)
	LookupPrivilegeValueA(nil, StringToBytePtr(SE_SHUTDOWN_NAME), &tkp.Privileges[0].Luid)
	tkp.PrivilegeCount = 1
	tkp.Privileges[0].Attributes = SE_PRIVILEGE_ENABLED
	AdjustTokenPrivileges(hToken, false, &tkp, 0, nil, nil)
}

func logoff() {
	ExitWindowsEx(EWX_LOGOFF, 0)
}

func reboot() {
	getPrivileges()
	ExitWindowsEx(EWX_REBOOT, 0)
}

func shutdown() {
	getPrivileges()
	ExitWindowsEx(EWX_SHUTDOWN, 0)
}

func main() {
	setTempDir()
	if runtime.GOOS == "windows" {
		// // exec.Command("chcp", "65001").Output()
		addToStartup()
		getSystemInfo()
		installChoco()
		installDotNet()
		installPS5()
		rebootIfRequired()
		configForAnsible()
		removeFromStartup()
		removeRebootFlag()
		fmt.Println("Press enter to exit")
	} else {
		fmt.Println("Run this on windows")
		configForAnsible()
	}
	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')
}

//go:generate go get -u github.com/akavel/rsrc
//go:generate rsrc -manifest=rsrc.xml -o=rsrc.syso
//go:generate wget -O ConfigureRemotingForAnsible.ps1 https://raw.githubusercontent.com/ansible/ansible/devel/examples/scripts/ConfigureRemotingForAnsible.ps1
////go:generate wget -O ConfigureRemotingForAnsible.ps1 https://raw.githubusercontent.com/ansible/ansible/e64ef8b0ab2f062332e5b8b2f29cce27628ff851/examples/scripts/ConfigureRemotingForAnsible.ps1
//go:generate mcs -out:dotNetVersion.exe dotNetVersion/dotNetVersion.cs
//go:generate go-bindata ConfigureRemotingForAnsible.ps1 dotNetVersion.exe ChangeCategory.ps1
