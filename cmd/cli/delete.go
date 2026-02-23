package main

type DeleteCmd struct {
	Args []string `arg:"" optional:"" help:"args for delete"`
}

func (cmd *DeleteCmd) Run(g *Globals) error {

	sendRequestDelete(g.BaseURL)

	return nil
}

func sendRequestDelete(baseURL string) {

}
