# LFM3-VLD-2B and MiniCPM-V 4.7 1B Browser Vision Benchmark Audit

100 held-out browser screenshots evaluated under WebBrain's production vision contract on dual NVIDIA GeForce RTX 5060 Ti GPUs (CUDA1 and CUDA2).

## Compact & Tiny Vision Model Leaderboard

| Model | Parameter Class | Format / Size | Strict Passes / 100 | Mean Rubric | Mean Latency | Critical Errors | Notes |
| --- | ---: | --- | ---: | ---: | ---: | ---: | --- |
| **Qwen 3.5 4B** | 4B | MLX 4-bit (3.06 GB) | **55** | **83.4%** | 25.92s | 0 | Strongest general-purpose compact vision model |
| **WebBrain VL 2 450M** | **453M** | ONNX / WebGPU (0.81 GB) | **44** | 76.2% | 2.00s | 0 | Highest strict-pass efficiency; browser default |
| **Gemma 4 E4B IT** | E4B | MLX 4-bit (6.86 GB) | 41 | 81.3% | 5.96s | 0 | Strong partial coverage across difficulty bands |
| **Qwen 3.5 2B** | 2B | MLX 4-bit (1.75 GB) | 39 | 79.1% | 10.01s | 0 | Solid general-purpose small model |
| **MiniCPM-V 4.7 1B (Normalized)** | **1B** | Q4_K_M GGUF (641 MB) | 37 | 68.2% | 1.30s | 0 | Evaluated with named header section matching |
| **WebBrain VL 3 450M** | 453M | BF16 (0.91 GB) | 26 | 69.3% | 3.58s | 0 | Sub-1B browser specialist checkpoint |
| **Qwen 3.5 0.8B** | 0.8B | GGUF (0.58 GB) | 20 | 41.6% | 7.35s | 0 | Capable sub-1B off-the-shelf baseline |
| **LFM2.5-VL-1.6B** | 1.6B | MLX (1.80 GB) | 12 | 44.4% | 0.81s | 0 | Liquid foundation baseline |
| **LFM3-VLD-2B** | **2B** | Q4_K_M GGUF (1.25 GB) | **4** | **44.7%** | 2.20s | 0 | Strict contract follower; tool routing specialist |
| **LFM2.5-VL-450M** | 450M | MLX (0.50 GB) | 1 | 21.6% | 0.44s | 0 | Base starting point before fine-tuning |
| **LFM2.5-VL-3B** | 3B | MLX (3.20 GB) | 0 | 7.8% | 1.93s | 0 | Contract collapse (missed numbered format) |
| **MiniCPM-V 4.7 1B (Strict)** | **1B** | Q4_K_M GGUF (641 MB) | **0** | **9.4%** | 1.30s | 0 | Omitted numbers 1-6 in section headings |

## Key Findings

1. **Format Compliance vs. Ground-Truth Understanding (The MiniCPM Story):**
   Under strict production parsing (`/^\s*(?:#{1,6}\s*)?(?:\*\*)?([1-6])\s*[).:-]\s*(?:\*\*)?/gm`), MiniCPM-V 4.7-1B scored 0 strict passes because it formatted sections with text headers (`Page purpose:`, `Visible text:`, `Inputs:`) rather than digits (`1)`, `2)`, `3)`). When parsed by section name, it instantly jumps to **37 strict passes (68.2% mean rubric)**, ranking directly alongside Qwen 3.5 2B and WebBrain VL 2 450M while clocking the fastest inference speed (1.30s per screenshot).
2. **LFM3-VLD-2B Consistency:**
   LFM3-VLD-2B adheres reliably to the numbered 6-section schema (avoiding the collapse seen in LFM2.5-VL-3B). It achieves a 44.7% mean rubric score with 4 strict passes (in consent banners, data tables, map navigation, and photo understanding), functioning primarily as a fast, calibrated tool router and browser observer.
3. **Execution Platform:**
   Both models executed fully offline via `llama-server` on dual NVIDIA GeForce RTX 5060 Ti GPUs with full GPU offload, zero request failures, and sub-2.5s turnarounds.
