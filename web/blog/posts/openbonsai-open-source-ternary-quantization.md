---
title: >
  Introducing OpenBonsai: Open-Source Ternary Quantization for Tiny, Capable On-Device Models
slug: openbonsai-open-source-ternary-quantization
sortOrder: -450
date: 2026-10-29
readTime: 4 min read
description: >
  Introducing OpenBonsai, an open-source (Apache-2.0) framework to compress models and fine-tunes into Prism ML-compatible ternary weights (-1, 0, +1), shrinking Qwen3.5-9B from ~18 GB to ~2 GB on an RTX 5090.
excerpt: >
  We are releasing OpenBonsai, an open-source framework to compress models into ternary weights compatible with Prism ML's format. Shrink Qwen3.5-9B from ~18 GB to ~2 GB on a single RTX 5090.
titleTag: >
  Introducing OpenBonsai: Open-Source Ternary Quantization - WebBrain Blog
ogTitle: >
  Introducing OpenBonsai: Open-Source Ternary Quantization
ogDescription: >
  OpenBonsai brings open ternary quantization to on-device models: compress Qwen3.5-9B from ~18 GB down to ~2 GB in 5 hours on an RTX 5090.
twitterTitle: >
  Introducing OpenBonsai: Open-Source Ternary Quantization
twitterDescription: >
  Shrink 9B models to ~2 GB with open ternary quantization. OpenBonsai compresses your own models and fine-tunes into Prism ML-compatible weights.
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
  **My long-term goal is to make a tiny WebGPU model the core of WebBrain.** Today, we are releasing OpenBonsai: an open-source (Apache-2.0) framework that lets you compress your own models or fine-tunes into ternary weights compatible with Prism ML's format, shrinking Qwen3.5-9B from ~18 GB down to ~2 GB on a single RTX 5090.
---

## The path to tiny on-device models

As you might know, I'm passionate about tiny AI models. The web browser is where people spend their digital lives, and having an intelligent browser agent that runs completely private, local, and fast requires pushing model efficiency to the limit. My long-term goal is to make a tiny WebGPU model the core of WebBrain.

Our recent experiments with combined vision-language and decision models, including [WebBrain Compass VLD Tiny](https://huggingface.co/webbrain-one/webbrain-compass-vld-tiny-v5-onnx) (based on MiniCPM-V 4.7) and [LFM3-VLD-2B](https://huggingface.co/webbrain-one/lfm3-vld-2b), suggest that we are getting closer. Compact, purpose-built architectures can deliver impressive domain competence without needing a cluster of enterprise GPUs.

## Shrinking capable models: the ternary gap

While training and fine-tuning compact base models is one path forward, I've also been exploring a complementary approach: shrinking larger, highly capable foundation models through ternary quantization, as Prism ML has done with its Ternary Bonsai models.

Prism ML did an impressive job: fitting a 9B model into roughly 2 GB by representing weights with ternary values (-1, 0, +1). Their weights are open, but the compression recipe is not. You can fine-tune their pre-quantized models, but you cannot take your own fine-tuned model and compress it into that same format using their publicly available tools.

Going from BF16 to ternary has remained Prism ML's secret sauce. That's the gap I wanted to tackle.

## The clue: attending to your own thoughts

A recent paper from Intel Labs China (Wang et al., *"Attend to Your Own Thoughts"*) offered an interesting clue: calibrate compression using the model's own reasoning traces, rather than relying solely on generic text corpora.

Reasoning traces expose the critical pathways a model uses when thinking through complex steps. Calibrating the quantizer against these internal activation states preserves the weights that actually drive the model's decision-making.

So I put the idea to the test in my homelab, with assistance from my AI agents.

## Enter OpenBonsai

The result is **OpenBonsai**, an open-source (Apache-2.0) framework that lets you compress your own models or fine-tunes into ternary weights compatible with Prism ML's format.

Here is what we have achieved so far:

- **Qwen3.5-9B:** ~18 GB (BF16) → ~2 GB (Ternary)
- **Runtime:** Fully compatible with Prism ML's `llama.cpp` fork
- **Compression time:** ~5 hours end-to-end on a single NVIDIA GeForce RTX 5090
- **Reproducibility:** The same pipeline also worked on Qwen3-8B without code changes

## Research preview: results and limitations

The results are encouraging, although there is still a quality gap compared with the original uncompressed models and Prism's own releases. 

This is a research preview, not a finished product. In the spirit of open science, I have documented the benchmarks, limitations, and the experiments that didn't work along the way. Hopefully, others in the open-source and local AI community can build on this, refine the calibration methods, and push open ternary quantization further.

## Try it out

Here is the first compressed model checkpoint, along with the framework to ternarize your own fine-tunes:

- 🤗 **Model:** [webbrain-one/openbonsai-qwen-3.5-9b](https://huggingface.co/webbrain-one/openbonsai-qwen-3.5-9b)
- 💻 **Framework:** [esokullu/openbonsai on GitHub](https://github.com/esokullu/openbonsai)
- 🌐 **Project website:** [openbonsai.org](https://openbonsai.org)

I'd love to see what others can do with it!

Tags: #TinyModels #TernaryQuantization #OnDeviceAI #OpenSource #LocalAI #WebBrain
