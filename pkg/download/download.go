package download

type DownloadProvider interface {
	Download() error
}

type Download struct {
	option Option
}

type Option struct {
	SnapshotId     string
	UserName       string
	Password       string
	CloudName      string
	CloudRegion    string
	DownloadPath   string
	CloudApiMirror string
}

func (d *Download) Download(opt Option) error {
	return nil
}
