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

### 2. Data Structure from generate_cfd_data Tool

The MCP Server's `generate_cfd_data` tool returns CFD data in the following structure:

```json
{
	"cfd_data": {
		"buckets": [
			{
				"date": "2025-08-29T00:00:00+02:00",
				"label": "2025-08-29",
				"by_issue_type": {
					"Story": {
						"Open": 56,
						"refining": 10,
						"developing": 7,
						"Done": 890
					},
					"Bug": {
						"Open": 11,
						"awaiting development": 2
					},
					"Activity": {
						"Open": 15,
						"refining": 4,
						"developing": 3,
						"Done": 100
					},
					"Defect": {
						"Open": 1
					}
				}
			},
			{
				"date": "2025-08-30T00:00:00+02:00",
				"label": "2025-08-30",
				"by_issue_type": {}
			}
		],
		"statuses": [
			"Closed",
			"Done",
			"Open",
			"UAT (+Fix)",
			"awaiting UAT",
			"awaiting deploy to Prod",
			"awaiting deploy to QA",
			"awaiting development",
			"deploying to Prod",
			"deploying to QA",
			"developing",
			"refining"
		],
		"availableIssueTypes": ["Activity", "Bug", "Defect", "Story"]
	}
}
```

**Key Structure Details:**

- **buckets**: Array of daily snapshots, one entry per day with label for chart display
- **by_issue_type**: Nested object where each issue type contains status → count mappings for that day
- **statuses**: List of all unique statuses found across the entire dataset
- **availableIssueTypes**: List of all issue types present in the data
- **label**: Human-readable date string (YYYY-MM-DD format, ideal for chart X-axis)

**Critical Transformation Requirement:**

The raw response has counts per issue type per status. **You must aggregate across issue types** to get the total count per status:

```javascript
const transformCFDData = (cfdResponse) => {
	const { buckets, availableIssueTypes } = cfdResponse.cfd_data;

	return buckets.map((bucket) => {
		const entry = { date: bucket.label };

		// Aggregate across all issue types for each status
		const statusTotals = {};
		Object.keys(bucket.by_issue_type).forEach((issueType) => {
			const typeData = bucket.by_issue_type[issueType];
			Object.keys(typeData).forEach((status) => {
				statusTotals[status] = (statusTotals[status] || 0) + typeData[status];
			});
		});

		// Add aggregated counts to entry
		Object.assign(entry, statusTotals);

		// Preserve issue type breakdown for filtering
		entry.issueTypes = {};
		availableIssueTypes.forEach((type) => {
			if (bucket.by_issue_type[type]) {
				entry.issueTypes[type] = Object.values(bucket.by_issue_type[type]).reduce(
					(sum, count) => sum + count,
					0
				);
			}
		});

		return entry;
	});
};
```

**Transformed output (ready for charting):**

```javascript
{
  date: "2025-08-29",
  Open: 83,
  refining: 14,
  developing: 10,
  Done: 990,
  issueTypes: {
    Story: 73,
    Bug: 13,
    Activity: 32,
    Defect: 1
  }
}
```

**Important Notes on Data:**

1. **Sparse Data**: Not every status appears for every issue type on every day. Missing status/type combinations are simply omitted (not included as 0).
2. **Status Variance by Project**: Different projects have different status sets. Use the `statuses` array returned by the tool, don't hardcode them.
3. **Issue Type Variance**: Similarly, `availableIssueTypes` varies by project. Don't assume a fixed set like "Story, Bug, Task, Epic".
4. **No Need for Manual Normalization**: The data already reflects current counts, not cumulative. Zero-count statuses are simply absent from the response.

### 3. Data Normalization

Normalize the "Done" baseline to show net delivery over the time period:

```javascript
const baselineDone = data[0]?.Done || 0;
const normalizedData = data.map((entry) => ({
	...entry,
	Done: entry.Done - baselineDone
}));
```

This transforms absolute delivery numbers into a relative "items delivered since Day 1" metric, making trends more visible and understandable.

## Implementation Guide

### Step 1: Set Up Imports

```javascript
import React, { useState, useMemo } from "react";
import { ComposedChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from "recharts";
```

**Do NOT use Legend from Recharts** for CFDs with toggle functionality—use a custom legend instead (see Step 7).

### Step 2: Define Status Arrays

Create **two separate arrays** for the dual ordering requirement:

### Static Configuration (Single Project)

```javascript
const statusesForChart = [
	"Done",
	"deploying to Prod",
	"awaiting deploy to Prod",
	"UAT (+Fix)",
	"awaiting UAT",
	"deploying to QA",
	"awaiting deploy to QA",
	"developing",
	"awaiting development",
	"refining",
	"Open"
];

const statusesForLegend = [
	"Open",
	"refining",
	"awaiting development",
	"developing",
	"awaiting deploy to QA",
	"deploying to QA",
	"awaiting UAT",
	"UAT (+Fix)",
	"awaiting deploy to Prod",
	"deploying to Prod",
	"Done"
];
```

### Dynamic Configuration (Multi-Project)

For projects with varying status lists, derive the reversed array programmatically:

```javascript
// Receive statuses in forward order from API or config
const statusesForLegend = [
	"Open",
	"refining",
	"awaiting development",
	"developing",
	"awaiting deploy to QA",
	"deploying to QA",
	"awaiting UAT",
	"UAT (+Fix)",
	"awaiting deploy to Prod",
	"deploying to Prod",
	"Done"
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
	"Open": "#ef4444", // Red
	"refining": "#f97316", // Orange
	"awaiting development": "#eab308", // Yellow
	"developing": "#22c55e", // Green
	"awaiting deploy to QA": "#06b6d4", // Cyan
	"deploying to QA": "#0ea5e9", // Sky
	"awaiting UAT": "#3b82f6", // Blue
	"UAT (+Fix)": "#8b5cf6", // Violet
	"awaiting deploy to Prod": "#d946ef", // Fuchsia
	"deploying to Prod": "#ec4899", // Pink
	"Done": "#10b981" // Emerald
};
```

**Design principle**: Use a vibrant, distinct color for each status. Avoid grey for active statuses; reserve grey (#64748b) for disabled statuses only.

### Dynamic Color Assignment

For projects with variable status lists, dynamically assign colors:

```javascript
const colorPalette = [
	"#ef4444",
	"#f97316",
	"#eab308",
	"#22c55e",
	"#06b6d4",
	"#0ea5e9",
	"#3b82f6",
	"#8b5cf6",
	"#d946ef",
	"#ec4899",
	"#10b981",
	"#14b8a6",
	"#6366f1",
	"#f43f5e",
	"#a855f7"
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
	statuses: ["Open", "In Progress", "Done"],
	colors: {
		"Open": "#ff6b6b",
		"In Progress": "#4ecdc4",
		"Done": "#45b7d1"
	}
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
const issueTypes = ["Story", "Bug", "Activity", "Defect"];
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
	setSelectedTypes((prev) => (prev.includes(type) ? prev.filter((t) => t !== type) : [...prev, type]));
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

### Step 5: Transform and Prepare Data

First, transform the raw `generate_cfd_data` response into chart-ready format:

```javascript
const transformCFDData = (cfdResponse) => {
	const { buckets, availableIssueTypes } = cfdResponse.cfd_data;

	return buckets.map((bucket) => {
		const entry = { date: bucket.label };

		// Step 1: Aggregate across issue types for each status
		const statusTotals = {};
		Object.keys(bucket.by_issue_type).forEach((issueType) => {
			const typeData = bucket.by_issue_type[issueType];
			Object.keys(typeData).forEach((status) => {
				statusTotals[status] = (statusTotals[status] || 0) + typeData[status];
			});
		});

		// Step 2: Add aggregated counts to entry
		Object.assign(entry, statusTotals);

		// Step 3: Preserve issue type breakdown for filtering
		entry.issueTypes = {};
		availableIssueTypes.forEach((type) => {
			if (bucket.by_issue_type[type]) {
				entry.issueTypes[type] = Object.values(bucket.by_issue_type[type]).reduce(
					(sum, count) => sum + count,
					0
				);
			}
		});

		return entry;
	});
};

// Transform the raw data once
const transformedData = useMemo(() => {
	return transformCFDData(cfdResponse);
}, [cfdResponse]);
```

Then, use `useMemo` to filter and normalize:

```javascript
const filteredData = useMemo(() => {
	if (!transformedData || transformedData.length === 0) return [];

	// 1. Apply issue type filtering (if applicable)
	const scaled = transformedData.map((entry) => {
		const totalInTypes = selectedTypes.reduce((sum, type) => sum + (entry.issueTypes[type] || 0), 0);
		const totalAllTypes = Object.values(entry.issueTypes || {}).reduce((a, b) => a + b, 0);
		const scaleFactor = totalAllTypes > 0 ? totalInTypes / totalAllTypes : 1;

		const filtered = { date: entry.date };
		Object.keys(entry).forEach((key) => {
			if (key !== "date" && key !== "issueTypes" && typeof entry[key] === "number") {
				filtered[key] = Math.round(entry[key] * scaleFactor);
			}
		});
		return filtered;
	});

	// 2. Normalize: subtract first day's Done value for relative delivery
	const baselineDone = scaled[0]?.Done || 0;
	return scaled.map((entry) => ({
		...entry,
		Done: entry.Done - baselineDone
	}));
}, [transformedData, selectedTypes]);
```

**Two Transformation Steps:**

1. **Transform**: Aggregate the nested `by_issue_type` structure into flat status counts (required)
2. **Filter & Normalize**: Apply issue type filtering and subtract Done baseline (optional but recommended)

**Why normalize?** Normalization removes absolute delivery numbers and shows relative delivery since Day 1, making delivery trends and process changes much more visible.

### Step 6: Render the Chart

```javascript
<ResponsiveContainer width="100%" height={550}>
	<ComposedChart data={filteredData} margin={{ top: 10, right: 30, left: 0, bottom: 20 }}>
		<CartesianGrid strokeDasharray="3 3" vertical={true} />
		<XAxis dataKey="date" />
		<YAxis />
		<Tooltip />

		{statusesForChart.map(
			(status) =>
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
		)}
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
<div
	style={{
		marginTop: "1.5rem",
		padding: "1rem",
		display: "flex",
		flexWrap: "wrap",
		justifyContent: "center",
		gap: "1rem"
	}}
>
	{statusesForLegend.map((status) => (
		<button
			key={status}
			onClick={() => toggleStatusVisibility(status)}
			style={{
				padding: "0.5rem 1rem",
				background: "transparent",
				border: `2px solid ${statusColors[status]}`,
				color: visibleStatuses.has(status) ? statusColors[status] : "#64748b",
				borderColor: visibleStatuses.has(status) ? statusColors[status] : "#64748b",
				borderRadius: "6px",
				cursor: "pointer",
				fontSize: "0.9rem",
				opacity: visibleStatuses.has(status) ? 1 : 0.5,
				transition: "all 0.2s ease"
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
