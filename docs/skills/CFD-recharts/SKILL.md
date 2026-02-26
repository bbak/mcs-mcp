---
name: cfd-recharts
description: Create production-grade Cumulative Flow Diagrams (CFD) with React and Recharts for Agile/Kanban workflow visualization. Includes data normalization, interactive filtering, dark theme styling, and proper workflow stacking.
license: Complete terms in LICENSE.txt
---

# Cumulative Flow Diagram (CFD) with React & Recharts

This skill guides the creation of professional Cumulative Flow Diagrams using React and Recharts. CFDs are essential for visualizing work-in-progress (WIP) across workflow statuses and identifying process bottlenecks.

## What is a Cumulative Flow Diagram?

A CFD is a line chart that visualizes the cumulative count of work items in each workflow status over time. Each status is represented as a stacked area, with the vertical distance between lines representing WIP at that status. CFDs help identify:

- **Bottlenecks**: Wide vertical bands indicate statuses where work accumulates
- **Flow patterns**: Steep lines indicate fast-moving work; flat lines indicate stalled work
- **Delivery trends**: Overall growth shows increasing WIP; flat/declining trends show consistent or improving delivery
- **Process health**: A healthy CFD shows balanced progression without dramatic accumulations

## Core CFD Architecture

### 1. Status Ordering (Critical)

A CFD **must** have two distinct status orderings:

**Chart Stacking Order** (bottom → top):
- Start with the **terminal status** (Done) at the bottom
- Progress **backwards** through the workflow
- End with the **first status** (Open/Backlog) at the top

Example: `['Done', 'deploying to Prod', 'awaiting deploy to Prod', ..., 'refining', 'Open']`

**Legend Display Order** (left → right):
- Start with the **first status** (Open/Backlog) on the left
- Progress **forward** through the workflow
- End with the **terminal status** (Done) on the right

Example: `['Open', 'refining', 'awaiting development', ..., 'deploying to Prod', 'Done']`

This dual ordering is **essential**: the chart needs reverse stacking to properly represent flow progression, while the legend needs forward ordering for intuitive understanding of workflow progression.

### 2. Data Structure Requirements

Each data point must contain:

```javascript
{
  date: "2026-02-01",
  Open: 45,              // Current count of items in "Open"
  refining: 12,          // Current count of items in "refining"
  'awaiting development': 28,
  // ... one property per status
  Done: 890,             // Cumulative delivered items
}
```

**Important**: Each property represents the **current count** at that status on that date, not cumulative totals (except for Done, which represents total delivered).

### 3. Data Normalization

Normalize the "Done" baseline to show net delivery over the time period:

```javascript
const baselineDone = data[0]?.Done || 0;
const normalizedData = data.map((entry) => ({
  ...entry,
  Done: entry.Done - baselineDone,
}));
```

This transforms absolute delivery numbers into a relative "items delivered since Day 1" metric, making trends more visible and understandable.

## Implementation Guide

### Step 1: Set Up Imports

```javascript
import React, { useState, useMemo } from 'react';
import {
  ComposedChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts';
```

**Do NOT use Legend from Recharts** for CFDs with toggle functionality—use a custom legend instead (see Step 7).

### Step 2: Define Status Arrays

Create **two separate arrays** for the dual ordering requirement:

### Static Configuration (Single Project)

```javascript
const statusesForChart = [
  'Done',
  'deploying to Prod',
  'awaiting deploy to Prod',
  'UAT (+Fix)',
  'awaiting UAT',
  'deploying to QA',
  'awaiting deploy to QA',
  'developing',
  'awaiting development',
  'refining',
  'Open',
];

const statusesForLegend = [
  'Open',
  'refining',
  'awaiting development',
  'developing',
  'awaiting deploy to QA',
  'deploying to QA',
  'awaiting UAT',
  'UAT (+Fix)',
  'awaiting deploy to Prod',
  'deploying to Prod',
  'Done',
];
```

### Dynamic Configuration (Multi-Project)

For projects with varying status lists, derive the reversed array programmatically:

```javascript
// Receive statuses in forward order from API or config
const statusesForLegend = [
  'Open',
  'refining',
  'awaiting development',
  'developing',
  'awaiting deploy to QA',
  'deploying to QA',
  'awaiting UAT',
  'UAT (+Fix)',
  'awaiting deploy to Prod',
  'deploying to Prod',
  'Done',
];

// Automatically create the reversed array for chart stacking
const statusesForChart = [...statusesForLegend].reverse();
```

This ensures the two arrays are **always exact opposites** regardless of the project's workflow. The workflow definition should always be:
1. Fetched from an API/config
2. Stored as `statusesForLegend` (forward order: from first status to Done)
3. Reversed to create `statusesForChart` (for stacking: Done at bottom)

**Critical**: These arrays must be **exact opposites** of each other. The chart uses `statusesForChart`; the legend uses `statusesForLegend`.

### Step 3: Create Color Mapping

Define a color for each status:

```javascript
const statusColors = {
  Open: '#ef4444',                    // Red
  refining: '#f97316',                // Orange
  'awaiting development': '#eab308',  // Yellow
  developing: '#22c55e',              // Green
  'awaiting deploy to QA': '#06b6d4', // Cyan
  'deploying to QA': '#0ea5e9',       // Sky
  'awaiting UAT': '#3b82f6',          // Blue
  'UAT (+Fix)': '#8b5cf6',            // Violet
  'awaiting deploy to Prod': '#d946ef', // Fuchsia
  'deploying to Prod': '#ec4899',     // Pink
  Done: '#10b981',                    // Emerald
};
```

**Design principle**: Use a vibrant, distinct color for each status. Avoid grey for active statuses; reserve grey (#64748b) for disabled statuses only.

### Dynamic Color Assignment

For projects with variable status lists, dynamically assign colors:

```javascript
const colorPalette = [
  '#ef4444', '#f97316', '#eab308', '#22c55e', '#06b6d4',
  '#0ea5e9', '#3b82f6', '#8b5cf6', '#d946ef', '#ec4899',
  '#10b981', '#14b8a6', '#6366f1', '#f43f5e', '#a855f7',
];

const generateStatusColors = (statusList) => {
  const colors = {};
  statusList.forEach((status, index) => {
    colors[status] = colorPalette[index % colorPalette.length];
  });
  return colors;
};

const statusColors = generateStatusColors(statusesForLegend);
```

This approach:
- Cycles through a predefined palette if more statuses than palette colors
- Ensures different statuses always get distinct colors
- Works with any number of statuses (3 to 20+)

### Alternative: User-Defined Color Mapping

Allow projects to define custom colors via configuration:

```javascript
// From API/config
const projectConfig = {
  statuses: ['Open', 'In Progress', 'Done'],
  colors: {
    'Open': '#ff6b6b',
    'In Progress': '#4ecdc4',
    'Done': '#45b7d1',
  },
};

const statusColors = projectConfig.colors;
const statusesForLegend = projectConfig.statuses;
const statusesForChart = [...statusesForLegend].reverse();
```

This gives maximum flexibility while maintaining consistency.

### Step 4: Initialize State for Visibility Toggle

```javascript
const [visibleStatuses, setVisibleStatuses] = useState(new Set(statusesForLegend));

const toggleStatusVisibility = (status) => {
  const newVisible = new Set(visibleStatuses);
  if (newVisible.has(status)) {
    newVisible.delete(status);
  } else {
    newVisible.add(status);
  }
  setVisibleStatuses(newVisible);
};
```

**Important**: Initialize the Set with `statusesForLegend`, not `statusesForChart`. This ensures consistency with the legend display order.

### Dynamic Issue Type Configuration

Issue types may also vary by project. Handle them dynamically:

```javascript
// Static approach (if issue types are known)
const issueTypes = ['Story', 'Bug', 'Activity', 'Defect'];
const [selectedTypes, setSelectedTypes] = useState(issueTypes);

// Dynamic approach (from API/config)
const [issueTypes, setIssueTypes] = useState([]);
const [selectedTypes, setSelectedTypes] = useState([]);

useEffect(() => {
  // Fetch from project configuration
  const fetchProjectConfig = async (projectKey) => {
    const response = await fetch(`/api/projects/${projectKey}/config`);
    const { issueTypes: types } = await response.json();
    setIssueTypes(types);
    setSelectedTypes(types); // Default: all selected
  };
  
  fetchProjectConfig(currentProjectKey);
}, [currentProjectKey]);

const toggleIssueType = (type) => {
  setSelectedTypes((prev) =>
    prev.includes(type) ? prev.filter((t) => t !== type) : [...prev, type]
  );
};
```

**Data structure for issue types in the data**:

Instead of hardcoding issue types, ensure each data point includes dynamic counts:

```javascript
// Example data point from API
{
  date: '2026-02-01',
  Open: 45,
  refining: 12,
  Done: 890,
  issueTypeCounts: {
    'Story': 150,
    'Bug': 45,
    'Activity': 120,
    'Defect': 25,
    // ... other types as defined by project
  }
}
```

This allows the filtering logic to work with any set of issue types without hardcoding.

### Step 5: Prepare and Filter Data

Use `useMemo` to recalculate filtered and normalized data when dependencies change:

```javascript
const filteredData = useMemo(() => {
  // 1. Apply issue type filtering (if applicable)
  const typeRatio = {};
  issueTypes.forEach((type) => {
    typeRatio[type] = selectedTypes.includes(type) ? 1 : 0;
  });

  const scaled = rawData.map((entry) => {
    const totalInTypes = selectedTypes.reduce(
      (sum, type) => sum + (entry.issueTypes[type] || 0),
      0
    );
    const scaleFactor =
      totalInTypes > 0
        ? selectedTypes.reduce((sum, type) => sum + (entry.issueTypes[type] || 0), 0) /
          Object.values(entry.issueTypes).reduce((a, b) => a + b, 0)
        : 1;

    const filtered = { date: entry.date };
    Object.keys(entry).forEach((key) => {
      if (key !== 'date' && key !== 'issueTypes' && typeof entry[key] === 'number') {
        filtered[key] = Math.round(entry[key] * scaleFactor);
      }
    });
    return filtered;
  });

  // 2. Normalize: subtract first day's Done value
  const baselineDone = scaled[0]?.Done || 0;
  return scaled.map((entry) => ({
    ...entry,
    Done: entry.Done - baselineDone,
  }));
}, [selectedTypes]);
```

**Why normalize?** Normalization removes absolute delivery numbers and shows relative delivery since Day 1, making delivery trends and process changes much more visible.

### Step 6: Render the Chart

```javascript
<ResponsiveContainer width="100%" height={550}>
  <ComposedChart data={filteredData} margin={{ top: 10, right: 30, left: 0, bottom: 20 }}>
    <CartesianGrid strokeDasharray="3 3" vertical={true} />
    <XAxis dataKey="date" />
    <YAxis />
    <Tooltip />

    {statusesForChart.map((status) => (
      visibleStatuses.has(status) && (
        <Area
          key={status}
          type="monotone"
          dataKey={status}
          stackId="1"
          stroke={statusColors[status]}
          fill={statusColors[status]}
          fillOpacity={0.7}
          strokeWidth={2}
          isAnimationActive={true}
          animationDuration={800}
        />
      )
    ))}
  </ComposedChart>
</ResponsiveContainer>
```

**Critical rendering rules**:
- Use `statusesForChart` for rendering, not `statusesForLegend`
- **Only render visible statuses**: `visibleStatuses.has(status) &&`
- Use `stackId="1"` to create proper stacking
- Conditional rendering ensures **gaps fill properly**: when a status is hidden, the statuses above it move down
- Always include `isAnimationActive` for smooth visual feedback

### Step 7: Create a Custom Legend (Crucial for Interactivity)

**Do NOT use Recharts' built-in Legend component** if you need toggle functionality. Instead, create a custom legend below the chart:

```javascript
<div style={{ 
  marginTop: '1.5rem', 
  padding: '1rem', 
  display: 'flex', 
  flexWrap: 'wrap', 
  justifyContent: 'center', 
  gap: '1rem' 
}}>
  {statusesForLegend.map((status) => (
    <button
      key={status}
      onClick={() => toggleStatusVisibility(status)}
      style={{
        padding: '0.5rem 1rem',
        background: 'transparent',
        border: `2px solid ${statusColors[status]}`,
        color: visibleStatuses.has(status) ? statusColors[status] : '#64748b',
        borderColor: visibleStatuses.has(status) ? statusColors[status] : '#64748b',
        borderRadius: '6px',
        cursor: 'pointer',
        fontSize: '0.9rem',
        opacity: visibleStatuses.has(status) ? 1 : 0.5,
        transition: 'all 0.2s ease',
      }}
    >
      {status}
    </button>
  ))}
</div>
```

**Why custom?** Recharts' Legend component filters items from its `data` prop, making it difficult to maintain a persistent legend where disabled items remain clickable. A custom legend using simple buttons solves this elegantly.

**Legend behavior**:
- Uses `statusesForLegend` for left-to-right workflow progression
- Buttons remain visible even when disabled (greyed out at 50% opacity)
- Click to toggle visibility; the chart updates immediately

### Step 8: Dark Theme Styling

Apply a dark theme with high contrast for readability:

```javascript
const darkThemeStyles = `
  body {
    background: #0f172a;
    color: #e2e8f0;
  }

  .recharts-cartesian-axis-tick-value {
    fill: '#94a3b8' !important;
    font-size: 0.85rem !important;
  }

  .recharts-cartesian-axis-line {
    stroke: rgba(148, 163, 184, 0.2) !important;
  }

  .recharts-cartesian-grid-horizontal line,
  .recharts-cartesian-grid-vertical line {
    stroke: rgba(148, 163, 184, 0.1) !important;
  }

  .recharts-tooltip-wrapper {
    outline: none;
  }

  .recharts-default-tooltip {
    background: rgba(15, 23, 42, 0.95) !important;
    border: 1px solid rgba(96, 165, 250, 0.5) !important;
    border-radius: 8px !important;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4) !important;
  }

  .recharts-tooltip-label {
    color: #60a5fa !important;
    font-weight: 600;
  }

  .recharts-tooltip-item {
    color: #cbd5e1 !important;
  }
`;
```

**Color palette for dark theme**:
- Background: `#0f172a` (dark blue)
- Text: `#e2e8f0` (light grey)
- Muted text: `#94a3b8` (medium grey)
- Disabled: `#64748b` (slate grey)
- Accent: `#60a5fa` (blue)

## Multi-Project Configuration Patterns

### Pattern 1: Configuration from API (Recommended)

Fetch all project-specific configuration from a single endpoint:

```javascript
const [projectConfig, setProjectConfig] = useState(null);
const [loading, setLoading] = useState(true);

useEffect(() => {
  const fetchConfig = async (projectKey) => {
    try {
      const response = await fetch(`/api/projects/${projectKey}/cfd-config`);
      const config = await response.json();
      
      // Validate that config contains required fields
      if (!config.statuses || !Array.isArray(config.statuses)) {
        throw new Error('Invalid config: statuses array required');
      }

      setProjectConfig({
        statuses: config.statuses, // Forward order
        issueTypes: config.issueTypes || [],
        colors: config.colors || generateStatusColors(config.statuses),
        dateRange: config.dateRange || { start: null, end: null },
      });
    } catch (error) {
      console.error('Failed to fetch config:', error);
      setProjectConfig(null);
    } finally {
      setLoading(false);
    }
  };

  if (projectKey) {
    fetchConfig(projectKey);
  }
}, [projectKey]);

// Use config throughout component
if (loading) return <div>Loading configuration...</div>;
if (!projectConfig) return <div>Failed to load project configuration</div>;

const statusesForLegend = projectConfig.statuses;
const statusesForChart = [...statusesForLegend].reverse();
const statusColors = projectConfig.colors;
const availableIssueTypes = projectConfig.issueTypes;
```

**API Response Shape**:
```json
{
  "statuses": [
    "Open",
    "In Progress",
    "Review",
    "Testing",
    "Done"
  ],
  "issueTypes": ["Story", "Bug", "Task", "Epic"],
  "colors": {
    "Open": "#ef4444",
    "In Progress": "#3b82f6",
    "Review": "#8b5cf6",
    "Testing": "#f59e0b",
    "Done": "#10b981"
  },
  "dateRange": {
    "start": "2026-01-01",
    "end": null
  }
}
```

### Pattern 2: Local Configuration Object

For simpler setups or testing:

```javascript
const projects = {
  'PROJ-A': {
    statuses: ['Open', 'In Progress', 'Done'],
    issueTypes: ['Story', 'Bug'],
    colors: {
      'Open': '#ef4444',
      'In Progress': '#3b82f6',
      'Done': '#10b981',
    },
  },
  'PROJ-B': {
    statuses: ['Backlog', 'Ready', 'Development', 'QA', 'Deployed'],
    issueTypes: ['Feature', 'Bugfix', 'Hotfix', 'Tech Debt'],
    colors: {
      'Backlog': '#94a3b8',
      'Ready': '#eab308',
      'Development': '#0ea5e9',
      'QA': '#8b5cf6',
      'Deployed': '#10b981',
    },
  },
};

const projectConfig = projects[selectedProject];
const statusesForLegend = projectConfig.statuses;
const statusesForChart = [...statusesForLegend].reverse();
const statusColors = projectConfig.colors;
```

### Pattern 3: Workflow Metadata from Jira

If using Jira, extract statuses from the project's workflow:

```javascript
const fetchJiraWorkflow = async (projectKey) => {
  // Get all statuses used in the project
  const statusResponse = await fetch(`/jira/rest/api/3/project/${projectKey}/statuses`);
  const statuses = await statusResponse.json();

  // Get issue types
  const issuesResponse = await fetch(`/jira/rest/api/3/project/${projectKey}/issue-types`);
  const issueTypes = await issuesResponse.json();

  // Filter to only statuses actually used in board workflow
  const boardStatuses = statuses
    .filter((s) => s.statusCategory && s.statusCategory.key !== 'undefined')
    .map((s) => s.name)
    .sort(); // Sort alphabetically or by custom order

  return {
    statuses: boardStatuses,
    issueTypes: issueTypes.map((it) => it.name),
  };
};

// Usage
const config = await fetchJiraWorkflow('IESFSCPL');
const statusesForLegend = config.statuses;
const statusesForChart = [...statusesForLegend].reverse();
```

### Validation Pattern

Always validate configuration before using:

```javascript
const validateConfig = (config) => {
  const errors = [];

  if (!config.statuses || !Array.isArray(config.statuses) || config.statuses.length === 0) {
    errors.push('Config must include non-empty statuses array');
  }

  if (!config.statuses.includes('Done')) {
    errors.push('Status list must include a terminal status named "Done"');
  }

  if (config.colors) {
    config.statuses.forEach((status) => {
      if (!config.colors[status]) {
        errors.push(`Missing color for status: ${status}`);
      }
    });
  }

  return {
    isValid: errors.length === 0,
    errors,
  };
};

const validation = validateConfig(projectConfig);
if (!validation.isValid) {
  console.error('Invalid configuration:', validation.errors);
  // Handle error gracefully
}
```

### Issue Type Filtering

If your data includes issue type breakdowns, add filtering:

```javascript
const issueTypes = ['Story', 'Bug', 'Activity', 'Defect'];
const [selectedTypes, setSelectedTypes] = useState(issueTypes);

// In data preparation:
const totalInTypes = selectedTypes.reduce(
  (sum, type) => sum + (entry.issueTypes[type] || 0),
  0
);
const scaleFactor = totalInTypes > 0
  ? selectedTypes.reduce((sum, type) => sum + (entry.issueTypes[type] || 0), 0) /
    Object.values(entry.issueTypes).reduce((a, b) => a + b, 0)
  : 1;
```

### Date Range Selection

Allow users to focus on specific time periods:

```javascript
const [dateRange, setDateRange] = useState({ start: null, end: null });

const filteredByDate = filteredData.filter((entry) => {
  const entryDate = new Date(entry.date);
  return (!dateRange.start || entryDate >= dateRange.start) &&
         (!dateRange.end || entryDate <= dateRange.end);
});
```

### Export to CSV/JSON

Provide data export for further analysis:

```javascript
const exportToCSV = () => {
  const csv = [
    ['Date', ...statusesForLegend].join(','),
    ...filteredData.map((row) =>
      [row.date, ...statusesForLegend.map((s) => row[s])].join(',')
    ),
  ].join('\n');
  
  const blob = new Blob([csv], { type: 'text/csv' });
  const url = window.URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = 'cfd-export.csv';
  a.click();
};
```

## Common Pitfalls & Solutions

### Pitfall 1: Inverted Status Order
**Problem**: Chart shows statuses in forward order instead of backward
**Solution**: Use `statusesForChart` (reversed) for chart rendering; `statusesForLegend` (forward) for legend only

### Pitfall 2: Disabled Statuses Disappear from Legend
**Problem**: When a status is hidden, it vanishes from the legend and can't be re-enabled
**Solution**: Use a custom legend with persistent buttons that remain visible when disabled (at reduced opacity)

### Pitfall 3: Gaps Don't Fill When Disabling a Status
**Problem**: Hiding a status leaves a blank space instead of statuses above filling the gap
**Solution**: Use conditional rendering with `visibleStatuses.has(status) &&` so only visible statuses are included in the stacked area rendering

### Pitfall 4: Legend Order Doesn't Match Workflow Flow
**Problem**: Legend shows Done → Open instead of Open → Done
**Solution**: Explicitly use `statusesForLegend` array with forward ordering for the legend; never rely on Recharts' automatic legend generation for CFDs

### Pitfall 5: Data Not Normalizing Properly
**Problem**: Delivery numbers dominate the chart, hiding WIP patterns
**Solution**: Subtract the first day's Done value from all Done values to show relative delivery

### Pitfall 6: Chart Height Too Small
**Problem**: Tooltip or legend gets cut off
**Solution**: Set ResponsiveContainer height to at least 550px; use bottom margin of 20-80px depending on legend size

## Real Data Integration

### Jira Integration Pattern

```javascript
const fetchCFDData = async (boardId, startDate, endDate) => {
  const response = await fetch(`/api/jira/cfd/${boardId}`, {
    params: { startDate, endDate },
  });
  const data = await response.json();
  return data.map((point) => ({
    date: point.date,
    Open: point.statusCounts.Open,
    refining: point.statusCounts.refining,
    // ... map all statuses
    Done: point.statusCounts.Done,
    issueTypeCounts: point.issueTypeCounts,
  }));
};
```

### Multi-Project Integration (Recommended)

For applications supporting multiple projects, create a generalized fetch function:

```javascript
const fetchProjectCFDData = async (projectKey, startDate, endDate) => {
  // Step 1: Fetch project configuration
  const configResponse = await fetch(`/api/projects/${projectKey}/cfd-config`);
  const projectConfig = await configResponse.json();

  // Step 2: Fetch CFD data
  const dataResponse = await fetch(`/api/projects/${projectKey}/cfd-data`, {
    params: { startDate, endDate },
  });
  const rawData = await dataResponse.json();

  // Step 3: Transform data to match configured statuses
  const transformedData = rawData.map((point) => {
    const transformed = { date: point.date };
    
    // Dynamically map all statuses from config
    projectConfig.statuses.forEach((status) => {
      transformed[status] = point.statusCounts[status] || 0;
    });

    // Include issue types if available
    if (point.issueTypeCounts) {
      transformed.issueTypeCounts = point.issueTypeCounts;
    }

    return transformed;
  });

  return {
    config: projectConfig,
    data: transformedData,
  };
};

// Usage
const { config, data } = await fetchProjectCFDData('PROJ-KEY', startDate, endDate);
const statusesForLegend = config.statuses;
const statusesForChart = [...statusesForLegend].reverse();
const statusColors = config.colors;
const filteredData = filterAndNormalizeData(data, statusesForLegend);
```

### API Response Shape

Your backend should return data points with dynamic statuses:

**Configuration endpoint** (`/api/projects/{projectKey}/cfd-config`):
```json
{
  "statuses": [
    "Open",
    "In Progress",
    "Review",
    "Testing",
    "Done"
  ],
  "issueTypes": ["Story", "Bug", "Task", "Epic"],
  "colors": {
    "Open": "#ef4444",
    "In Progress": "#3b82f6",
    "Review": "#8b5cf6",
    "Testing": "#f59e0b",
    "Done": "#10b981"
  }
}
```

**Data endpoint** (`/api/projects/{projectKey}/cfd-data`):
```json
[
  {
    "date": "2026-02-01",
    "statusCounts": {
      "Open": 45,
      "In Progress": 28,
      "Review": 12,
      "Testing": 8,
      "Done": 890
    },
    "issueTypeCounts": {
      "Story": 50,
      "Bug": 20,
      "Task": 15
    }
  },
  // ... more data points
]
```

**Key principle**: Status names in the response must exactly match those in `projectConfig.statuses`. This ensures seamless mapping regardless of how many statuses a project has.

## Performance Optimization

### Memoization

Use `useMemo` for expensive calculations:

```javascript
const filteredData = useMemo(() => {
  // Filtering and normalization logic
}, [selectedTypes, dateRange]);
```

This prevents unnecessary recalculations when unrelated state changes.

### Large Datasets

For datasets with 1000+ data points:
- Consider sampling (e.g., every 7th day)
- Use `dontAnimate` on Area components
- Increase ResponsiveContainer height to reduce rendering pressure

## Testing & Validation

### Unit Tests for Data Normalization

```javascript
test('normalizes Done baseline correctly', () => {
  const data = [
    { date: '2026-02-01', Open: 10, Done: 100 },
    { date: '2026-02-02', Open: 12, Done: 105 },
  ];
  const normalized = normalizeData(data);
  expect(normalized[0].Done).toBe(0);
  expect(normalized[1].Done).toBe(5);
});
```

### Visual Regression Testing

- Capture baseline screenshots with all statuses visible
- Test with various visibility combinations
- Verify gap-filling behavior when statuses are hidden

## Best Practices

1. **Always normalize Done**: Helps identify delivery velocity patterns
2. **Provide status filtering**: Let users focus on specific workflow stages
3. **Use consistent colors**: Same status should always be same color
4. **Label axes clearly**: Include units (e.g., "Items", "Work Units")
5. **Include time context**: Show date range and granularity (daily, weekly)
6. **Offer export options**: Enable data-driven discussions with exported CFD data
7. **Update frequency**: Refresh CFD data daily for meaningful trend analysis
8. **Accessibility**: Ensure sufficient color contrast; don't rely on color alone to differentiate

## Resources

- Recharts Documentation: https://recharts.org/
- Cumulative Flow Diagram Theory: https://en.wikipedia.org/wiki/Cumulative_flow_diagram
- Kanban Analytics & Flow Metrics: https://businessmap.io/kanban-resources/kanban-tutorial/analytics
