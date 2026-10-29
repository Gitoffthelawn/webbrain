---
title: >
  Tiny Vision Models Compared III: LFM3-VLD and MiniCPM-V 4.7 Under WebBrain's Strict Browser Rubric
slug: tiny-vision-models-compared-iii
sortOrder: -445
date: 2026-10-29
readTime: 9 min read
description: >
  Official 100-case browser-vision benchmark evaluating LFM3-VLD-2B and MiniCPM-V 4.7 1B locally on dual NVIDIA RTX 5060 Ti GPUs. Format compliance, ground-truth visual comprehension, and local sub-2B latency.
excerpt: >
  Testing LFM3-VLD-2B and MiniCPM-V 4.7 1B on WebBrain's frozen 100-case browser viewport vision suite reveals a tale of two models: strict formatting discipline versus raw visual perception.
titleTag: >
  Tiny Vision Models Compared III: LFM3-VLD and MiniCPM-V 4.7 - WebBrain Blog
ogTitle: >
  Tiny Vision Models Compared III: LFM3-VLD & MiniCPM-V 4.7
ogDescription: >
  How do the newest compact vision models perform under WebBrain's 100-case screenshot benchmark? LFM3-VLD-2B and MiniCPM-V 4.7 evaluated locally on RTX 5060 Ti GPUs.
twitterTitle: >
  LFM3-VLD & MiniCPM-V 4.7 tested on WebBrain's browser vision suite
twitterDescription: >
  A 100-case browser vision shootout: format compliance, OCR accuracy, and local latency across dual NVIDIA RTX 5060 Ti GPUs.
keywords:
  - MiniCPM-V 4.7
  - LFM3-VLD
  - Liquid AI
  - OpenBMB
  - browser vision
  - vision benchmark
  - NVIDIA RTX 5060 Ti
  - llama.cpp
  - WebBrain
  - multimodal
  - on-device AI
author: Emre Sokullu
authorUrl: https://emresokullu.com
lede: >
  **A tiny multimodal model can have sharp visual perception and still fail a brittle agent contract.** In our newest 100-case browser viewport shootout, [webbrain-one/lfm3-vld-2b](https://huggingface.co/webbrain-one/lfm3-vld-2b) and [openbmb/MiniCPM-V-4.7-1B](https://huggingface.co/openbmb/MiniCPM-V-4.7-1B-gguf) went head-to-head on dual NVIDIA RTX 5060 Ti GPUs. LFM3-VLD-2B proved disciplined, adhering strictly to the six-section contract with 4 full passes and a 44.7% mean rubric score. MiniCPM-V 4.7 scored 0 strict passes under our exact numbered regex because it dropped the leading digits `1)` to `6)` in favor of markdown labels; yet when scored on its actual section content, MiniCPM shot up to **37 strict passes and a 68.2% mean score** at blazing 1.3-second latency.
---

## The expanded compact vision table

Following our earlier [Tiny Vision Models Compared II](/blog/tiny-vision-models-compared-ii) report, we evaluated the newest generation of compact multimodal models released in late 2026. 

Both new models were run entirely offline in 4-bit quantization (Q4_K_M GGUF) using `llama-server` on dedicated NVIDIA GeForce RTX 5060 Ti GPUs. Every model evaluated in this benchmark encountered the identical 100 held-out browser screenshots (ranging from clean landing pages to dense analytics dashboards, multilingual checkouts, and occluded modals), the exact production system prompt, and the deterministic scoring rubric.

| Model | Parameter class | Artifact size | Strict passes / 100 | Mean rubric | Inference latency | Platform |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| **[Qwen 3.5 4B](https://huggingface.co/Qwen/Qwen3.5-4B)** | 4B | 3.06 GB | **55/100** | **83.4%** | 25.9s | MLX 4-bit |
| **[WebBrain VL 2 450M](https://huggingface.co/webbrain-one/webbrain-vl-2-450M)** | **453M** | **0.81 GB** | **44/100** | 76.2% | 2.0s | ONNX / WebGPU |
| **[Gemma 4 E4B IT](https://huggingface.co/google/gemma-4-E4B-it)** | E4B | 6.86 GB | 41/100 | 81.3% | 6.0s | MLX 4-bit |
| **[Qwen 3.5 2B](https://huggingface.co/Qwen/Qwen3.5-2B)** | 2B | 1.75 GB | 39/100 | 79.1% | 10.0s | MLX 4-bit |
| **[MiniCPM-V 4.7 1B](https://huggingface.co/openbmb/MiniCPM-V-4.7-1B-gguf)** *(semantic parsing)* | **1B** | **0.64 GB** | **37/100** | **68.2%** | **1.3s** | GGUF Q4 (RTX 5060 Ti) |
| **[WebBrain VL 3 450M](https://huggingface.co/webbrain-one/webbrain-vl-3-450M)** | 453M | 0.91 GB | 26/100 | 69.3% | 3.6s | BF16 PyTorch |
| **[Qwen 3.5 0.8B](https://huggingface.co/Qwen/Qwen3.5-0.8B)** | 0.8B | 0.58 GB | 20/100 | 41.6% | 7.4s | GGUF |
| **[LFM2.5-VL-1.6B](https://huggingface.co/LiquidAI/LFM2.5-VL-1.6B)** | 1.6B | 1.80 GB | 12/100 | 44.4% | 0.8s | MLX |
| **[LFM3-VLD-2B](https://huggingface.co/webbrain-one/lfm3-vld-2b)** *(official contract)* | **2B** | **1.25 GB** | **4/100** | **44.7%** | **2.2s** | GGUF Q4 (RTX 5060 Ti) |
| **[LFM2.5-VL-450M](https://huggingface.co/LiquidAI/LFM2.5-VL-450M)** | 450M | 0.50 GB | 1/100 | 21.6% | 0.4s | MLX |
| **[MiniCPM-V 4.7 1B](https://huggingface.co/openbmb/MiniCPM-V-4.7-1B-gguf)** *(strict contract)* | 1B | 0.64 GB | 0/100 | 9.4% | 1.3s | GGUF Q4 (RTX 5060 Ti) |
| **[LFM2.5-VL-3B](https://huggingface.co/LiquidAI/LFM2.5-VL-3B)** | 3B | 3.20 GB | 0/100 | 7.8% | 1.9s | MLX |

*All 100 cases scored with zero HTTP errors or timeouts. A strict pass requires full compliance with WebBrain's six-section observation schema without critical hallucinations.*

---

## The MiniCPM anomaly: why a 1B model scored 0% and 68% at the same time

When MiniCPM-V 4.7 finished its 100 cases, the initial score report looked disastrous: 0 strict passes, 9.4% mean rubric score.

Yet when we inspected the raw output files, the descriptions were astonishingly vivid. Consider case `001.png` (a standard account sign-in screen):

```text
Page purpose: WebScene sign-in dialog.
Visible text: "Welcome back", "Use your account to continue", "Email", "you@example.com", "Password", "Enter password", "Sign in", "Need help?"
Inputs: Email field with value "you@example.com", Password field with label "Enter password", blue "Sign in" button, blue "Need help?" link.
State signals: None visible.
Blockers: None.
Unknowns: None.
```

MiniCPM identified every single visible UI element, extracted the verbatim button text, recognized the password placeholder, and correctly verified that no modals or error banners blocked the viewport.

Why did the benchmark assign it 0%?

WebBrain's production vision prompt requests six numbered sections:
1. `1) Page purpose:`
2. `2) Visible text:`
3. `3) Inputs:`
4. `4) State signals:`
5. `5) Blockers:`
6. `6) Unknowns:`

To prevent cross-section hallucinations (e.g. counting an error message found in a photo background as a UI blocker), WebBrain's production grader isolates each section using a strict regex:
```javascript
const marker = /^\s*(?:#{1,6}\s*)?(?:\*\*)?([1-6])\s*[).:-]\s*(?:\*\*)?/gm;
```

MiniCPM-V 4.7 provided all six sections, with the exact heading names, in the exact order—but omitted the numeral prefix `1)` to `6)`. As a result, the parser found zero numbered sections, scoped checks evaluated to empty strings, and critical checks failed.

When we normalized the section headers to recognize `Page purpose:`, `Visible text:`, and `Inputs:` regardless of whether a leading digit was present, MiniCPM's score transformed:
- **Strict passes:** 0/100 &rarr; **37/100**
- **Mean rubric:** 9.4% &rarr; **68.2%**
- **Speed:** **1,299 ms** average on an RTX 5060 Ti

At 641 MB for the quantized weights, MiniCPM-V 4.7 demonstrates visual acuity close to Qwen 3.5 2B (39 passes) with half the parameters and five times faster inference. But for autonomous agents that rely on structured scrapers, it highlights an important lesson: **format compliance is as critical as perception.**

---

## LFM3-VLD-2B: disciplined structure from a decision specialist

Liquid AI's LFM3 architecture brings a hybrid linear computational foundation to compact multimodal tasks. We tested [webbrain-one/lfm3-vld-2b](https://huggingface.co/webbrain-one/lfm3-vld-2b), a 2B checkpoint paired with its multimodal vision projector (`mmproj-LFM3-VL-2B-VLDR-F16.gguf`).

Unlike its predecessor LFM2.5-VL-3B—which completely collapsed on the numbered prompt structure—LFM3-VLD-2B followed the six-section format faithfully across all 100 test queries.

- **Strict passes:** 4/100 (in consent banners, data tables, navigation maps, and photo understanding)
- **Mean rubric score:** 44.7%
- **Average latency:** 2,199 ms on RTX 5060 Ti
- **Errors:** 0

Examining LFM3-VLD's responses across difficulty tiers shows steady reliability:

| Difficulty tier | Cases | Strict passes | Mean rubric | Mean latency |
| --- | ---: | ---: | ---: | ---: |
| **Easy** (001–020) | 20 | 2 | 51.2% | 1.70s |
| **Basic** (021–040) | 20 | 2 | 50.6% | 2.05s |
| **Intermediate** (041–060) | 20 | 0 | 44.6% | 2.04s |
| **Advanced** (061–080) | 20 | 0 | 32.3% | 3.10s |
| **Challenging** (081–100) | 20 | 0 | 44.8% | 2.10s |

LFM3-VLD was fine-tuned primarily as a **Vision-Language Decision (VLD)** model—aimed at compact tool routing, next-action selection, and action ground-truthing in WebBrain's browser loop (where it scored a joint #1 36.0% loose routing match in our parallel tool-routing benchmark). 

When repurposed as an end-to-end screen describer, it reliably identifies major form inputs, active tabs, and primary blockers, even if it leaves out minor background snippets that our 100-case rubric rewards.

---

## Category breakdown

Our 100-case benchmark spans twenty real-world UI categories:

![Category breakdown for LFM3-VLD and MiniCPM-V 4.7 across 20 browser UI categories](/assets/browser-vision-benchmark-strict-passes.png)

| Category | LFM3-VLD-2B (Strict) | MiniCPM-V 4.7 (Semantic) | What the test stresses |
| --- | ---: | ---: | --- |
| **Photo understanding** | **62.2%** | 22.2% | Natural scene context, image labels |
| **Modal overlay** | **61.8%** | 71.4% | Identifying blocking backdrop and close buttons |
| **Uncertainty calibration** | **59.3%** | 50.0% | Refusing to hallucinate partially obscured data |
| **Map and travel** | **58.2%** | 42.9% | Pins, route directions, zoom controls |
| **Data table** | **53.3%** | 68.6% | Associating columns with row headers |
| **Chart reading** | 47.5% | **65.0%** | Reading axes, bar heights, trendlines |
| **Search results** | 45.8% | **73.3%** | Result counts, snippets, search queries |
| **Email compose** | 44.2% | **62.5%** | To/CC fields, attachments, rich-text toolbar |
| **Toast notifications** | 41.8% | **58.3%** | Transient popups, auto-dismiss banners |
| **Multilingual OCR** | 32.7% | **54.5%** | Non-Latin scripts (Japanese, Cyrillic, Arabic) |
| **Form validation** | 26.0% | **64.0%** | Red error borders, inline error captions |
| **Checkout & payment** | 28.4% | **61.5%** | Card forms, billing address toggles, totals |

In categories dominated by fine-grained UI text reading (Checkout, Form validation, Search results, and Charts), MiniCPM-V 4.7's vision encoder exhibits higher text fidelity, correctly capturing validation errors and currency amounts.

In contextual reasoning and restraint (Uncertainty calibration, Modal detection, and Photo scenes), LFM3-VLD is more cautious, declining to guess unreadable fields and accurately flagging overlay states.

---

## Hardware footprint on NVIDIA RTX 5060 Ti

Both evaluations were conducted on dual consumer **NVIDIA GeForce RTX 5060 Ti (16 GB)** graphics cards running native `llama-server` binaries with full layer offloading (`-ngl 99`):

- **MiniCPM-V 4.7 1B:**
  - Base model: 641 MB (Q4_K_M)
  - Multimodal projector: 1.11 GB (F16)
  - Total VRAM allocated: ~2.4 GB
  - Prompt evaluation: ~32 ms/token (multimodal projection ~700 ms)
  - Generation speed: ~28 tokens/sec
  - Total latency per screenshot: **1.30s**
- **LFM3-VLD-2B:**
  - Base model: 1.25 GB (Q4_K_M)
  - Multimodal projector: 379 MB (F16)
  - Total VRAM allocated: ~2.8 GB
  - Prompt evaluation: ~35 ms/token (multimodal projection ~1.2s)
  - Generation speed: ~128 tokens/sec
  - Total latency per screenshot: **2.20s**

Both models operate well within the memory headroom of consumer desktop GPUs and can comfortably co-exist with a local language model or background browser automation instance.

---

## Practical takeaways for agent builders

1. **Guard against format fragility in small vision models:** Small models under 2B parameters frequently drop structural conventions (such as list numbering or specific prefixes) when their attention is split between visual encoding and textual generation. Agent frameworks should prefer regex parsers that match both numbered and named section headers.
2. **Specialized vs. Generalist trade-offs:** LFM3-VLD-2B is optimized as an action router—a role where it excels at deciding click/type targets. MiniCPM-V 4.7 is a strong general-purpose visual reader whose OCR and visual grounding punch well above its 1B weight class.
3. **Local vision is viable for consumer hardware:** Both models achieve sub-2.5 second screenshot observations locally on an RTX 5060 Ti with under 3 GB of VRAM, making cloud vision APIs unnecessary for local browser agents.
