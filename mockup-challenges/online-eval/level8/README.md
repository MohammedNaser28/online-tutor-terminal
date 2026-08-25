# Level #8

Git

## Level Description

1. Linus Trovalds is not better than us! we made our own 'version control' tool called `lit`! (don't tell but it's just git).

2. Find the `lit-pull.sh` script and run it to fetch remote changes.

3. After running it you'll find some files in your current working directory representing some commands.

4. First edit the `project.java` file and add the text "System.out.println("lit is lit!")" to it.

5. then ***in the right order as if you were using git commands*** pass the correctly named files to `lit.sh` to:
	- add your changes to the staging area.
	- commit to your responsibilties and those changes!
	- push them from local to remote.

> [!NOTE]	
> Make sure to pass them all as arguments in the correct order and in one line, do not run the script with one filename one at a time.

## Example Run

./lit.sh command1 command2 command3

---

> [!IMPORTANT]
> Don't forget to make all your scripts executable

---

> [!NOTE]
> After running `lit.sh`, validate your work with `./check.sh add commit push`
