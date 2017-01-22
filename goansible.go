package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

var envRegex = regexp.MustCompile(`%[^%]+%`)
var choco = "C:/ProgramData/chocolatey/bin/choco.exe"

func CheckError(err error) {
	if err != nil {
		// fmt.Println("Error: ", err)
		fmt.Fprintf(os.Stderr, "Error: : %s", err)
	}
}

func CheckErrorFatal(err error) {
	if err != nil {
		panic(fmt.Sprintf("Error: ", err))
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

var ps5url = "https://download.microsoft.com/download/3/F/D/3FD04B49-26F9-4D9A-8C34-4533B9D5B020/Win7AndW2K8R2-KB3066439-x64.msu"
var tempdir string

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
		fmt.Println("Installing choco...")
		_, err := exec.Command("powershell", "iex ((new-object net.webclient).DownloadString('https://chocolatey.org/install.ps1'))").Output()
		if err != nil {
			fmt.Println("Error installing choco: ", err)
		}
	} else {
		fmt.Println("Choco is already installed")
	}
}

func installDotNet() {
	fmt.Println("Checking if dotnet4.6 is installed")
	fmt.Println("Installing or upgrading dotnet4.6...")
	out, err := exec.Command(choco, "upgrade", "-y", "dotnet4.6").Output()
	fmt.Println(string(out))
	if err != nil {
		fmt.Println("error: ", err)
	}
}

func installPS5() {
	fmt.Println("Checking powershell version if any")
	url := ps5url
	uris := strings.Split(url, "/")
	pkg := uris[len(uris)-1]
	path := tempdir + pkg
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
				fmt.Printf("Downloading %s to %s\n", pkg, path)
				downloadFile(path, url)
				out, err := exec.Command("cmd", "/c "+path+" /quiet").Output() // try /q /norestart for dotnet
				fmt.Println("out: ", string(out), ", err: ", err)
			}
		}
	}

	removeFileIfExist(path)
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
	out, err := exec.Command("powershell", "Set-ExecutionPolicy", "Bypass").Output()
	if err != nil {
		fmt.Println("Cannot set powershell execution policy to 'Bypass': ", err)
	} else {
		fmt.Println(string(out))
		fmt.Println("Preparing host for ansible...")
		out, err := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", path).Output()
		// out, err := exec.Command("cmd", "/c powershell -ExecutionPolicy Bypass -File "+path).Output()
		if err != nil {
			fmt.Println("Failed to preparing host for ansible: ", err)
			fmt.Println("Stdout: ", string(out))
		} else {
			fmt.Println(string(out))
		}
	}
	removeFileIfExist(path)
}

func main() {
	setTempDir()
	if runtime.GOOS == "windows" {
		// installDotNet()
		configForAnsible()
		installChoco()
		installPS5()
		fmt.Println("Press enter to exit")
	} else {
		fmt.Println("Run this on windows")
		configForAnsible()
	}
	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')
}

//go:generate go get -u github.com/akavel/rsrc
//go:generate rsrc -manifest=rsrc.xml -o="rsrc.syso"
//go:generate wget -c https://raw.githubusercontent.com/ansible/ansible/devel/examples/scripts/ConfigureRemotingForAnsible.ps1
//go:generate go-bindata ConfigureRemotingForAnsible.ps1
