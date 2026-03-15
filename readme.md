# Garage trip chores

[![Go](https://github.com/gdg-garage/garage-trip-chores/actions/workflows/go.yml/badge.svg)](https://github.com/gdg-garage/garage-trip-chores/actions/workflows/go.yml)

Discord bot which:
* Tracks chores and who finished them (with global stats)
* Assigns (mentions) potentional asignees (Discord members) to the chores (based on who worked the lowest amount of time)
* Scheduled chores

Additional features:
* Temporarily disable members for scheduling (implemented using Discord role membership)
  * **Track presence** of the users
* Chores are marked as done (also may be explicitly rejected)
  * Support editing the chore length
* Manually add finished chores
* Display the track record
* Display the global statistics
* Use LLM to make the messages funny
* Users have **capabilities** and some chores needs expertise - use Discord roles to assign 
* Tries to find new asignees when not acknowledged by the original ones

### Slash commands
* Documeted in the sever commands itself including all the params

### Use cases
* Urgent tasks - use 🌶️🌶️🌶️ (1-3) in the chore name
* Delayed tasks (I need something done tonight) - Adjust the deadline accordingly, set the assignment timeout to 0 (disabled)
* Manually add finished chore - create chore, ACK yourself (=volunteer) and mark as done
* Scheduled tasks may be created via API (e.g. from a local script or LLM)

### Tech stack
* Golang
  * [slog](https://pkg.go.dev/log/slog)
  * [gorm](https://github.com/go-gorm/gorm)
  * [discordgo](https://github.com/bwmarrin/discordgo)
  * [viper](https://github.com/spf13/viper)
  * [chi](https://github.com/go-chi/chi)
  * [huma](https://github.com/danielgtaylor/huma)
  * [gorilla/websocket](https://github.com/gorilla/websocket)
* SQLite
* [JJ](https://github.com/jj-vcs/jj) (VCS)

### TODO
- [ ] Proactive stats sharing with LLM integration
  - Solved via dashboard.
- [ ] refuse ACK if someone worked too much compared to others
  - Not the best idea - people should check stats and communicate.
