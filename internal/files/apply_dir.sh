#!/bin/bash
set -ex

mkdir -p ~/$ONCTLDIR
cd ~/$ONCTLDIR

# Set the starting number
max_num=-1

# Check directories starting with 'apply' in the current directory
for dir in apply[0-9][0-9]; do
    if [[ -d $dir ]]; then
        # Get the directory number
        num=${dir#apply}
        # Update the maximum number
        if ((10#$num > max_num)); then
            max_num=10#$num
        fi
    fi
done

# If there are no 'apply' directories, create apply00
if [[ max_num -eq -1 ]]; then
    mkdir apply00
    echo -n "apply00"
else
    # Calculate and format the next directory number
    next_num=$(printf "apply%02d" $((max_num + 1)))
    mkdir "$next_num"
    echo -n "$next_num"
fi
