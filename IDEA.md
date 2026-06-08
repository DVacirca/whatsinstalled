Developers install packages and dependencies through various methods (apt, npm, pip, conda, manual binaries, etc.). Given the amount of new tooling being developed it's tempting to want to install and test the latest, but it's also hard to keep track, building up a graveyard of tools that you may not need.

Enter 'installr' — a CLI/TUI that gives you an overall picture of the packages installed on your system. It can tell you the source it was installed from, when you last used it, and help you add and remove them. It even understands natural language queries like "which python based tools do i have available?" via a small local LLM.

Package managers supported:
- apt (system-wide Debian packages)
- snap (system-wide Ubuntu packages)
- npm (global + local node projects)
- pip (global + local Python venvs)
- conda (all environments)
- bin (manual binaries in user directories like ~/.local/bin)
