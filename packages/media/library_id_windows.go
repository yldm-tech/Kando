package media

import "syscall"

// identifyDir reads the volume serial number and the file index of the directory at path, which NTFS keeps when the directory is renamed on its volume, as os.SameFile compares them. The directory is opened only to read them, and shared for deleting too, so a rename at the same moment does not fail on it.
func identifyDir(path string) DirID {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return DirID{}
	}
	h, err := syscall.CreateFile(name, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return DirID{}
	}
	defer syscall.CloseHandle(h)
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil || info.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return DirID{}
	}
	return MakeDirID(uint64(info.VolumeSerialNumber), uint64(info.FileIndexHigh)<<32|uint64(info.FileIndexLow))
}
