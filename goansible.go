package main

import (
	"bufio"
	"errors"
	"fmt"
	. "github.com/CodyGuo/win"
	"github.com/kardianos/osext"
	"golang.org/x/sys/windows"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"syscall"
)

var envRegex = regexp.MustCompile(`%[^%]+%`)

const choco = "C:/ProgramData/chocolatey/bin/choco.exe"
const dotNetVersion = "4.6"
const chocoRebootCode = 3010

var tempdir string

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
	_, err = file.Write([]byte("start \"\" \"" + tempdir + exe + "\""))
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
		addToStartup()
		installChoco()
		ChocoInstall("dotnet" + dotNetVersion)
		ChocoInstall("powershell")
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
//go:generate go-bindata ConfigureRemotingForAnsible.ps1 ChangeCategory.ps1
