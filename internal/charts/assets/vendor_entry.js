// Vendor entry point: builds a single IIFE that exposes React, ReactDOM, and
// Recharts as window globals for consumption by chart templates.
import * as React from 'react';
import * as ReactDOM from 'react-dom/client';
import * as Recharts from 'recharts';

window.__MCS_VENDOR__ = { React, ReactDOM, Recharts };
