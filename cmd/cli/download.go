package main

type DownloadCmd struct {
	Args []string `arg:"" optional:"" help:"args for download"`
}

func (cmd *DownloadCmd) Run() error {
	return nil
}
