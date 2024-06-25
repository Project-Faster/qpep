//go:generate goversioninfo -64 versioninfo.json -o resource.syso
//go:generate cmd /c "copy /y resource.syso ..\\qpep-tray\\resource.syso"
//go:generate cmd /c "move /y resource.syso ..\\resource.syso"

package version
