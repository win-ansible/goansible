# goansible

Tools for initial ansible windows setup. Current goal is to support windows7+ OSes. Inspired by [these ps scripts](https://github.com/cchurch/ansible/blob/devel/examples/scripts/).

Currently implemented:
- installation of [chocolatey](https://chocolatey.org/) from the internet
- ~~installation of Microsoft .NET Framework 4.6 (4.5 or higher is required by WMF 5.0) using chocolatey~~
- installation of [WMF 5.0](https://www.microsoft.com/en-us/download/details.aspx?id=48729) with Win7AndW2K8R2-KB3066439-x64.msu downloaded from the internet, will reboot after installation
- execution of `ConfigureRemotingForAnsible.ps1` downloaded from [ansible repo](/ansible/ansible/blob/devel/examples/scripts/ConfigureRemotingForAnsible.ps1) and embedded into binary with `go genearte`.


##### Build

```bash
mkdir -vp ~/$GOPATH/bin/win/
go get -u github.com/jteeuwen/go-bindata/...
go generate
go get
GOARCH=386 GOOS=windows go build -o ~/$GOPATH/bin/win/goansible.exe
```

#### Tested on

- [x] Windows 10
