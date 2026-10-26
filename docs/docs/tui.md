---
sidebar_position: 3
---
# Terminal UI

`onctl tui` is your VM list as a live, full-screen view laid out like
[k9s](https://k9scli.io): a header with the provider, VM counts and the
current view's hotkeys, one table, and a crumbs line. It refreshes every 15
seconds, and every action in it runs the matching `onctl` command.

```bash
onctl tui                # the current provider (ONCTL_CLOUD, or -p)
onctl tui -p hetzner
```

![onctl tui: the VMs view](/img/tui/vms.png)

The screenshots on this page are the real view rendered against sample
Hetzner data, so the keys, columns and colours are what the command draws.

## Moving around

`j`/`k` or the arrows move the cursor, `g`/`G` jump to the top and bottom,
`ctrl-f`/`ctrl-b` page. `shift` + a column's letter sorts by that column, and
again the other way. `esc` goes back a page; `ctrl-c` or `:q` quits.

`?` lists every key for the view you're in:

![The help page](/img/tui/help.png)

## Filtering

`/` opens a filter prompt; the rows narrow as you type, and `esc` clears it.
In the VMs view, `0`, `1` and `2` show all VMs, the running ones or the
paused ones (the two sections of `onctl ls`).

![Filtering the VMs view](/img/tui/filter.png)

## Working on a VM

With the cursor on a VM:

| | |
|---|---|
| `s` | open a terminal on it (`onctl ssh`) |
| `a` | run Claude Code on it (`onctl claude --box <vm>`, see [Claude Code on a box](./claude)) |
| `d` or `enter` | describe it; `y` for its JSON |
| `c` | create a VM: a form asks name, image and type, then runs `onctl create` |
| `p` / `r` | pause / resume (`onctl pause`, `onctl resume`) |
| `ctrl-d` | destroy it (`onctl destroy`), after asking |

![Describing a VM](/img/tui/describe.png)

Destroy asks first:

![The destroy confirmation](/img/tui/destroy.png)

## Views

`:` opens a command prompt, with `tab` completion (`:pro` becomes
`:providers`). `:vms`, `:images`, `:sizes` and `:repos` switch view; `:vms
web` switches and filters in one go.

- `:images` lists the provider's OS images, the ones `--image` takes. `c` on
  a row creates a VM from it.
- `:sizes` lists the sizes `--type` takes, with vCPUs, memory, disk and,
  where the provider publishes them, prices. `c` on a row creates a VM of
  that size.

![The sizes view](/img/tui/sizes.png)

- `:repos` lists the GitHub repositories `gh`'s account can reach, newest
  push first. `enter` on one runs `onctl claude --repo owner/name`: a new box
  on this provider every time (`<name>-<word>-<word>-<id>`), with the
  repository cloned and Claude Code attached, so two pieces of work on a
  repository never share a box. The box is kept when Claude exits; pause or
  destroy it from `:vms`.

![The GitHub repos view](/img/tui/repos.png)

- `:provider` lists the clouds onctl knows, the current one marked; `enter`
  switches the tui to that one. `:provider hetzner` switches directly.
- `:create`, `:ssh <vm>` and `:claude [vm]` are the key actions as commands.
- `R` refreshes now. VMs refresh on their own every 15 seconds; images,
  sizes and repos are listed when their view opens and on `R`.

## Every key

```
:vms :images :sizes :repos   switch view (also :q, :help, :create)
:provider [name]      switch cloud provider (a list to pick from without a name)
:claude [vm]          run Claude Code on a VM (onctl claude --box vm; the project's box without one)
/text        filter the rows; esc clears
j/k, arrows  move; g/G top/bottom; shift+letter sorts by that column
enter, d     describe the VM; y its JSON
s            open a terminal on the VM (onctl ssh)
a            run Claude Code on the VM (onctl claude --box vm)
enter        in :repos (your GitHub repos, newest push first, from gh), start Claude
             on a box of this provider with the repo cloned (onctl claude --repo)
c            create a VM (name, image and type are asked; runs onctl create)
p / r        pause / resume
ctrl-d       destroy the VM (asks first)
0 / 1 / 2    all VMs / running only / paused only
esc          back; ctrl-c or :q quits
```
