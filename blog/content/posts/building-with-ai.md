---
title: "From prompt to intent engineering: what building a package manager taught me about managing AI"
date: 2026-03-07
description: "An engineering manager who hasn't coded in a decade shipped a package manager with AI. The hardest part wasn't the code -- it was getting AI to focus on the intent of the project instead of optimizing for completing its tasks."
author: "Dan Gazineu"
author_github: "dangazineu"
---

As an engineering manager, I haven't written production code as my day job in over a decade. I just shipped a package manager.

## A two-hour problem

Every couple of years, when I get a new machine, I spend hours installing all my favorite dev tools. I have shell scripts to automate this, but they always bitrot. Python tools that installed with pip now need pipx. Docker's apt repository signing instructions changed again. gh comes from Homebrew, Gemini CLI from npm, Claude Code from its own installer. Every time I fix the scripts, promise myself I'll maintain them, and forget about them for two years.

Last year the scripts broke again, as they always do, but this time I asked Claude Code to fix them. And then I asked it to standardize their flags, and then to make them testable, and then to make the steps declarative, and then to rewrite them in Go, and then... when I noticed I was writing a package manager.

By now, [tsuku](https://github.com/tsukumogami/tsuku) has recipes for over a thousand tools and can generate new ones for tools it's never seen, using an LLM. Evenings, weekends, and four months of Claude Code Max subscription later, I've automated two hours of toil with two hundred hours of engineering work.

What kept me going wasn't the need to build yet another package manager -- it was that making one work across Linux and macOS, handling glibc and musl, x86_64 and arm64, requires the kind of low-level systems knowledge I'd never exercised. Parsing binary headers to check if a download matches your platform. Fixing library paths so shared objects load properly. Preventing path traversal attacks during archive extraction.

I'd spent my career making services talk to each other across a network, but I never paid attention to the complexities of how they get installed on a machine. And that was the point. I wanted to see how far I could get on a domain I was not familiar with, letting the AI do most of the coding.

And while Claude was coding, I was learning to manage it. Focusing on getting correct results, and letting go of the implementation details that I couldn't evaluate anyway.

When I asked Claude to design how tsuku should handle system dependencies -- things like gcc and zlib that some tools need during compilation -- it came back with four separate dependency fields, pkg-config verification, and platform-specific package databases. I don't know enough about how system package managers wire up native libraries to have proposed a simpler alternative. But I know when I see unwarranted complexity. After a series of questions that boiled down to "why does it need to be this complicated?", the design pivoted four times in an evening. Midway through, it became clear the right abstraction was already there -- we had recipes for tools and ecosystems for resolution, and system packages were just tools whose package managers were just ecosystems. What started as additional complexity ended up as a simplification. A dramatic codebase refactor became "add a .toml file."

Just like when I became a manager, I eventually stopped assigning tasks, and started delegating projects. I converted my prompts into skills that allowed Claude to work through issues and milestones to completion. I could now leave it working over night or while I was at work, just to review the results, correct mistakes and draft another batch of work the next evening.

The key was structure: for each issue, the AI would evaluate what needed to be done, do the work, and validate that the goal of the issue was met. It didn't need me steering every step because the process told it how to progress and how to unblock itself.

This graduation from prompt engineering into context engineering didn't come without its challenges though.

I wanted tsuku to generate recipes on its own, without requiring an API key. So I instructed Claude to build a local inference pipeline in Rust -- download a model, load it into memory, generate tokens. Claude went deep. It designed the pipeline to support everything from 0.5B parameter models on CPU to large models on GPU, with format detection, memory management, and graceful fallbacks. The next morning, all tests were green, and Claude told me confidently that it worked.

I asked it to try generating a recipe, and it almost fried my CPU.

Claude had optimized for "can run inference," not "can produce recipes." A tiny model will happily generate syntactically valid TOML -- it just won't be a working recipe. CPU inference on anything large enough to be useful was impractically slow.

Every issue closed. Every test green. The pipeline still didn't work. Claude had executed my plan exactly how I described it, I just didn't describe it well enough, and it didn't know any better to catch it. Context engineering had gotten Claude to execute, but completing tasks isn't the same as achieving a goal. I needed Claude to understand the objective behind the project, I needed intent engineering.

As any manager knows, the answer to excellent execution in the wrong direction isn't to micromanage -- it's to change the conversation. So I promoted Claude to a senior role. Instead of giving it projects to complete, I started giving it problems to solve.

Concretely, that meant restructuring around problem statements instead of task descriptions. A bug report came in that the verify command crashed on libraries. The old approach would've fixed the crash -- add a file-existence check, close the issue. Under the new process, the design phase asked what "verify" should actually mean for a library. The answer wasn't "the file exists" but "the library will load at runtime" -- wrong architecture, missing dependencies, broken linker paths. The entire original fix became an optional flag. That reframing happened before any code was written.

I've worked with brilliant engineers who knew their field in far more depth than I ever will -- deep in the how, but not always connected to the why. The gap was never ability -- it was connection. They had knowledge the business needed but wouldn't surface on its own. My job was to frame the problem in a way they could understand and reliably own, because when they own the problem, they own the pivots needed to solve it. Automating that bridge is what intent engineering looks like in practice -- scaling it to AI, where the bottleneck is your ability to get the specialist to understand and act on intent.

It took me a lot longer to realize this part, but somewhere along the way I figured that what I was building wasn't just a package manager, it was a reusable workflow for AI assisted development -- but that part I have just started.

In the meantime, I can rest assured that my next laptop setup will take about thirty seconds.
