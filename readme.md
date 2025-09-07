# Garage trip chores

[![Go](https://github.com/gdg-garage/garage-trip-chores/actions/workflows/go.yml/badge.svg)](https://github.com/gdg-garage/garage-trip-chores/actions/workflows/go.yml)

Discord bot which:
* Tracks chores and who finished them (with global stats)
* Assigns (mentions) potentional asignees (Discord members) to the chores (based on who worked the lowest amount of time)
* Scheduled chores

Additional features:
* Temporarily disable members for scheduling (implemented using Discord role membership)
  * Track presence of the users
* Chores are marked as done (also may be explicitly rejected)
  * Support editing the chore length
* Manually add finished chores (create chore, assign yourself and mark as done)
* Display the track record
* Display the global statistics
* Use LLM to make the messages funny
* Users have **capabilities** and some chores needs expertise - use Discord roles to assign 

### Slash commands
* Documeted in the sever commands itself including all the params

### Use cases
* Urgent tasks - use 🌶️🌶️🌶️ (1-3) in the chore name
* Delayed tasks (I need something done tonight) - Adjust the deadline accordingly, set the assignment timeout to 0 (disabled)

### Tech stack
* Golang
  * [slog](https://pkg.go.dev/log/slog)
  * [gorm](https://github.com/go-gorm/gorm)
  * [discordgo](https://github.com/bwmarrin/discordgo)
  * [viper](https://github.com/spf13/viper)
* SQLite
* [JJ](https://github.com/jj-vcs/jj) (VCS)

### TODO
- [ ] Task scheduler (config based)
- [ ] Proactive stats sharing with LLM integration


