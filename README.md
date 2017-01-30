# goansible

Tools for initial ansible windows setup. Current goal is to support windows7+ OSes. Inspired by [these ps scripts](https://github.com/ansible/ansible/blob/devel/examples/scripts/).

Use with casion, checkout workflow and code first! Windows setup code is mostly stackoverflow-driven-developed.

##### workflow

- copy self to `%TEMP%`
- create `startGoansible.cmd` at `shell:startup` path pointing to bynary path from previous step. That's done to simplify execution when system is rebooted by installers
- install [chocolatey](https://chocolatey.org/)
- install .Net 4.6 with choco. If reboot is required after installation it's marked for later execution as further installation might also require it
- install [PS5](https://chocolatey.org/packages/PowerShell) with choco. If reboot is required after installation it's marked for later execution as further installation might also require it
- change all public networks type to private – WinRM won't start if there're public ones and next step will fail
- reboot if still required, manual logon required after it
- execute `ConfigureRemotingForAnsible.ps1` downloaded from [ansible repo](https://github.com/ansible/ansible/blob/devel/examples/scripts/ConfigureRemotingForAnsible.ps1)
- mark binary created on first step for removal on reboot
- remove `startGoansible.cmd`

##### build

```bash
mkdir -vp ~/$GOPATH/bin/win/
go get -u github.com/jteeuwen/go-bindata/...
go generate
go get
GOARCH=386 GOOS=windows go build -o $GOPATH/bin/win/goansible.exe
```

#### tested on

- [ ] Windows 7 x32
- [x] Windows 7 x64
- [ ] Windows 10 x32
- [x] Windows 10 x64
