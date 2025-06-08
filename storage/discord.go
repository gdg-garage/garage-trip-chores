package storage

import "github.com/bwmarrin/discordgo"

func discordConnect(token string) (*discordgo.Session, error) {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	err = dg.Open()
	if err != nil {
		return nil, err
	}

	return dg, nil
}
