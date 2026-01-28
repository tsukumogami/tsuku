# Issue 1038 Implementation Plan

## Overview

Documentation-only issue to update CONTRIBUTING.md with recipe separation guidance.

## Files to Modify

1. `CONTRIBUTING.md` - Add new sections

## Implementation Steps

### Step 1: Add Recipe Category Guidance

Insert after "Adding Recipes" header, before "Using Recipe Builders":
- Decision flowchart (text-based ASCII)
- Three-directory explanation table
- Reference to EMBEDDED_RECIPES.md

### Step 2: Add Troubleshooting Entries

Add to existing Troubleshooting section:
- "Recipe Works Locally But Fails in CI" entry
- "Recipe Not Found (Network Issues)" entry

### Step 3: Add Nightly Validation Documentation

Add new section "Nightly Registry Validation" after Troubleshooting:
- Explain the 2 AM UTC workflow
- What contributors should do when notified

### Step 4: Add Incident Response Playbook

Add new section "Security Incident Response":
- Detection methods
- Immediate actions
- Recovery steps
- Prevention measures

## Acceptance Criteria Mapping

| Requirement | Section |
|-------------|---------|
| Recipe category decision flowchart | Step 1 |
| Three recipe directories table | Step 1 |
| "Works locally fails in CI" troubleshooting | Step 2 |
| Network-related "recipe not found" troubleshooting | Step 2 |
| Nightly validation documented | Step 3 |
| Incident response playbook | Step 4 |
| References EMBEDDED_RECIPES.md | Step 1 |
