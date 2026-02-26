# Installing Skills

After downloading and extracting the MCP Server archive, follow these steps to install the skills in Claude.

**Note**: This may be slightly different for other Agents/Assistants that already support skills.

## Prerequisites

- Node.js installed (required to render JSX/React components from skills)

## Step 1: Find Your Claude Skills Directory

Depending on your operating system:

**Windows:**

```
C:\Users\YourUsername\AppData\Roaming\Claude\skills
```

**Mac:**

```
~/Library/Application Support/Claude/skills
```

**Linux:**

```
~/.config/Claude/skills
```

Create the `skills` folder if it doesn't exist.

## Step 2: Copy Skills

Copy all folders from the archive's `skills/` directory into your Claude skills directory.

For example, copy `cfd-recharts/`, `another-skill/`, etc. into:

- `C:\Users\...\Claude\skills\` (Windows)
- `~/Library/Application Support/Claude/skills/` (Mac)
- `~/.config/Claude/skills/` (Linux)

## Step 3: Restart Claude

Close and reopen Claude. The skills are now available.

## Troubleshooting

**Skills not appearing:**

- Verify skills are in the correct directory
- Make sure each skill folder contains a `SKILL.md` file
- Restart Claude

**On Mac/Linux, if you get permission errors:**

```bash
chmod -R 755 ~/.config/Claude/skills
```
