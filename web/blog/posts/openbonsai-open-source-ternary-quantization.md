---
title: >
  Introducing OpenBonsai: open-source ternary quantization for tiny on-device models
slug: openbonsai-open-source-ternary-quantization
sortOrder: -450
date: 2026-10-29
readTime: 4 min read
description: >
  OpenBonsai is an open-source Apache-2.0 framework for ternary quantization, compressing 9B models like Qwen3.5-9B from 18 GB down to ~2 GB on a single RTX 5090.
excerpt: >
  Prism ML showed that 9B models can fit into 2 GB with ternary weights, but the compression pipeline stayed closed. OpenBonsai is an Apache-2.0 recipe to ternarize your own models and fine-tunes.
titleTag: >
  Introducing OpenBonsai: Open-Source Ternary Quantization - WebBrain Blog
ogTitle: >
  Introducing OpenBonsai: open-source ternary quantization for on-device models
ogDescription: >
  OpenBonsai brings open ternary quantization to local AI: compress Qwen3.5-9B from ~18 GB down to ~2 GB in 5 hours on a single RTX 5090.
twitterTitle: >
  Introducing OpenBonsai: open-source ternary quantization
twitterDescription: >
  Compress 9B models to ~2 GB with open ternary quantization. OpenBonsai lets you ternarize your own models and fine-tunes with Prism ML compatibility.
keywords:
  - OpenBonsai
  - ternary quantization
  - tiny models
  - on-device AI
  - WebBrain
  - Qwen3.5-9B
  - Prism ML
  - Ternary Bonsai
  - llama.cpp
  - local AI
author: Emre Sokullu
authorUrl: https://emresokullu.com
lede: >
  **Our long-term target for WebBrain is an on-device WebGPU model running at the core of the browser agent.** Recent experiments with combined vision-language and decision models — including [WebBrain Compass VLD Tiny](https://huggingface.co/webbrain-one/webbrain-compass-vld-tiny-v5-onnx) and [LFM3-VLD-2B](https://huggingface.co/webbrain-one/lfm3-vld-2b) — show that compact architectures are getting close. But training small models from scratch isn't the only path: shrinking capable foundation models through ternary quantization opens another route entirely.
---

## The missing half of the ternary recipe

Prism ML demonstrated what ternary quantization can do with its Ternary Bonsai releases: packing a 9B model into roughly 2 GB of memory by constraining weights to ternary values `{-1, 0, +1}`.

The weights were released openly, but the compression pipeline stayed behind closed doors. You could fine-tune an existing ternary checkpoint, but if you trained your own BF16 model or specialized fine-tune, there was no public tool to compress it into the same format. Going from full precision down to ternary weights remained a proprietary black box.

That was the gap we set out to close.

## Calibrating against reasoning traces

A recent paper from Intel Labs China (Wang et al., *"Attend to Your Own Thoughts"*) provided the key insight.

Traditional post-training quantization evaluates weight importance across generic text corpora. But when a model is compressed down to ternary precision, generic calibration washes out the nuanced activation pathways that govern step-by-step reasoning. Wang et al. proposed a different approach: calibrate compression using the model's own reasoning traces.

By conditioning the quantizer on the internal activations generated during genuine problem solving, the pipeline protects the weights that drive decision-making and planning.

We implemented and iterated on this approach in our lab, using autonomous agent workflows to accelerate the experimental loop.

## What OpenBonsai delivers

The result is **OpenBonsai**, an Apache-2.0 framework that compresses arbitrary BF16 models and fine-tunes into ternary weights compatible with Prism ML's runtime.

The initial results on consumer hardware:

- **Qwen3.5-9B:** ~18 GB (BF16) compressed to ~2 GB in ternary format.
- **Runtime:** Direct drop-in compatibility with Prism ML's `llama.cpp` fork.
- **Hardware footprint:** End-to-end compression takes ~5 hours on a single NVIDIA GeForce RTX 5090.
- **Portability:** The exact same pipeline compressed Qwen3-8B without architecture-specific code tweaks.

## A research preview

This is a research preview, not a finished product. While the compressed models produce coherent generations at a fraction of the memory footprint, a measurable quality delta remains when compared against uncompressed base models and Prism ML's proprietary releases.

Rather than keeping the pipeline private until every benchmark was solved, we are publishing the code, the first checkpoint, and our failure logs. The repository documents our benchmarks, known failure modes, and calibration experiments that didn't pan out, giving the community a concrete baseline to build on.

## Resources and checkpoints

The framework and initial checkpoints are available today:

- 🤗 **Model checkpoint:** [webbrain-one/openbonsai-qwen-3.5-9b](https://huggingface.co/webbrain-one/openbonsai-qwen-3.5-9b)
- 💻 **Code & pipeline:** [esokullu/openbonsai on GitHub](https://github.com/esokullu/openbonsai) (Apache-2.0)
- 🌐 **Project documentation:** [openbonsai.org](https://openbonsai.org)

If you're exploring extreme quantization or running agents on edge hardware, give it a spin. Pull requests and replication reports are welcome.

Tags: #TinyModels #TernaryQuantization #OnDeviceAI #OpenSource #LocalAI #WebBrain
