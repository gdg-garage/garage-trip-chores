# Garage trip chores

[![Go](https://github.com/gdg-garage/garage-trip-chores/actions/workflows/go.yml/badge.svg)](https://github.com/gdg-garage/garage-trip-chores/actions/workflows/go.yml)

Discord bot which:
* Tracks chores and who finished them (with global stats)
* Assigns (mentions) potentional asignees (Discord members) to the chores (based on who worked the lowest amount of time)
* Scheduled chores

Additional features:
* Temporarily disable members for scheduling (implemented using Discord role membership)
* Chores are marked as done with an emoji (also may be explicitly rejected)
  * Support editing the chore length
* Manually add finished chores
* Display the track record
* Display the global statistics
* Use LLM to make the messages funny
* Users have capabilities and some chores needs expertise - use Discord roles to assign 

### Tech stack
* Golang
  * [slog](https://pkg.go.dev/log/slog)
  * [gorm](https://github.com/go-gorm/gorm)
  * [discordgo](https://github.com/bwmarrin/discordgo)
  * [viper](https://github.com/spf13/viper)
* SQLite
* [JJ](https://github.com/jj-vcs/jj) (VCS)

### TODO
- [x] fire emoji for urgent task (or peppers - 1-3 hotness) - no need to implement just document this and add the emoji to the task name
- [ ] reminders in private messages
- [ ] when someone is not present we need to take that to acccount (othewise they will get all the tasks)
