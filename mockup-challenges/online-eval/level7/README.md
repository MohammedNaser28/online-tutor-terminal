# Level #7

Find, Text processing

---

> [!NOTE]
> Run `check.sh` to get your key

---

## Level Description

# File Validation Task

## Requirements

1. Use `find` to search recursively from the current directory.
2. Find only regular files that are exactly 55 bytes in size.
3. For every matching file:
   - Extract the value from the `date:` line.
   - The date contains only a year.
4. Pass **both** person numbers from the matching files to `check.sh` **in ascending order**, where each extracted year is greater than 2000.

## Example File

name: Alex Carter
date: 2024
phone no: +1-202-555-0001
