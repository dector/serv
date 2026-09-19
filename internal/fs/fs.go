package fs

import "os"

type FsNode struct {
	Path string
	Info os.FileInfo

	IsFile      bool
	IsDirectory bool
}

func GetFsNode(path string) (*FsNode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	return &FsNode{
		Path: path,
		Info: info,

		IsFile:      info.Mode().IsRegular(),
		IsDirectory: info.IsDir(),
	}, nil
}
