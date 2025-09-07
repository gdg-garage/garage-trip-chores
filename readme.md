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
- [x] reminders in private message
  - [x] ping before timeout
  - [x] is the task already done (after deadline)? 
    * for users who acked the task
    * for the creator of the task
  - [x] how much time have you spent?
  - [x] delete task?
- [x] when someone is not present we need to take that to acccount (othewise they will get all the tasks)
- [x] list all open tasks
- [x] list my tasks
- [x] list users stats (work log, assign stats, presence stats, total)
- [x] How to create a delayed task?
  * we do not need this - added default deadline (task without deadline is useless), assignment timeout can be disabled.
- [x] Assignment timeouts (0 for disable)
- [x] Edit created task
- [ ] Task scheduler (config based)
- [ ] Proactive stats sharing with LLM integration


