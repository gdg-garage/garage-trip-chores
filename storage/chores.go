package storage

func AddChore(chore Chore) error {}

func EditChore(chore Chore) error {}

func CompleteChore(Id int) error {}

func GetChores() ([]Chore, error) {}

func GetCompletedChores() ([]Chore, error) {}
