# Level #9

Links
## Level Description

Your application needs to read a configuration file. It is programmed to look for a file named `app_config` directly in your current directory. 

However, the system administrator has stored the actual configuration file deep in the system at: **`temp/var/opt/app/hidden_config.cfg`**

Your mission:
1. **Do not** copy or move the original file. 
2. Create a **symbolic link** (soft link) in your current directory named `app_config` that points to the exact **absolute path** of the hidden config file.
3. Run `./check.sh` to validate your work and get your key.

> **Hint:** Check the `man` page for the `ln` command, and pay attention to the flag required for "symbolic" links.
