package autohotkey

import "golang.org/x/sys/windows/registry"

// installDirs are the folders AutoHotkey says it is installed in (InstallDir under
// Software\AutoHotkey): for the user first, then for the machine.
func installDirs() []string {
	var dirs []string
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
			k, err := registry.OpenKey(root, `Software\AutoHotkey`, registry.QUERY_VALUE|view)
			if err != nil {
				continue
			}
			if d, _, err := k.GetStringValue("InstallDir"); err == nil && d != "" {
				dirs = append(dirs, d)
			}
			k.Close()
		}
	}
	return dirs
}
