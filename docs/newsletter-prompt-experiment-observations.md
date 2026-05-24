# Newsletter prompt experiment observations

## Goal

Run a local experiment that creates a newsletter, scores it with a smaller LLM judge, feeds the judge feedback into a prompt improver, and repeats that loop for five iterations while tracking the score trajectory from the baseline.

## Initial repo observations

- The production newsletter synthesis path is in `backend/internal/knowledge/digest_synthesis.go`.
- The current digest prompt version is `newsletter-stratechery-story-v5`.
- The production `digest:run` command generates a digest, tries to generate an illustration, attempts delivery, and saves the result. That is too side-effectful for a prompt experiment.
- The experiment should keep source inputs fixed across all iterations. Otherwise score changes can come from input selection drift instead of prompt changes.
- The local fixture `data/runtime/latest-knowledge-run.json` exists and can be used without fetching fresh X or YouTube data.
- The judge should be a smaller model than the generator. The local harness defaults to `gpt-4o-mini` for judging and the repo's `OPENAI_SYNTHESIS_MODEL` for generation and prompt improvement.
- The prompt should not be rewritten in production during the experiment. The harness applies an in-memory experimental addendum, records it, and writes reports under `data/runtime/newsletter-experiments/`.

## Experiment design

- Iteration `0` is the baseline with the current newsletter prompt and no addendum.
- After each judged run except the final one, the improver receives the current addendum, generated newsletter, source input JSON, and judge feedback.
- The improver returns a compact addendum of 3-7 instructions.
- The next generation uses the original prompt plus that addendum.
- Each run records overall score, grounding, synthesis, editorial voice, usefulness, structure, source-linking, subject, judge rationale, requested improvements, and the addendum used for the next run.
- The command is `npm run newsletter:eval`.

## Run notes

- Local run completed at `2026-05-24T14:02:56Z`.
- Report JSON: `data/runtime/newsletter-experiments/newsletter-eval-20260524T140256Z.json`.
- Report markdown: `data/runtime/newsletter-experiments/newsletter-eval-20260524T140256Z.md`.
- The run used `data/runtime/latest-knowledge-run.json`, not a fresh source refresh and not email delivery.
- Input counts were 14 summaries, 0 insights, 0 themes, 0 insight clusters, and 0 connections. This means the experiment tested newsletter writing and prompt revision over summaries, but it did not exercise the richer insight/graph context.
- Score trajectory:

| Iteration | Overall | Grounding | Synthesis | Voice | Usefulness | Structure | Links | Subject |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 0 | 75.0 | 80.0 | 70.0 | 85.0 | 70.0 | 70.0 | 80.0 | The new startup default is no default |
| 1 | 80.0 | 80.0 | 75.0 | 85.0 | 80.0 | 80.0 | 85.0 | The advantage is no longer the default |
| 2 | 82.0 | 85.0 | 75.0 | 80.0 | 85.0 | 80.0 | 70.0 | When Startup Defaults Become the Risk |
| 3 | 82.0 | 85.0 | 80.0 | 85.0 | 80.0 | 75.0 | 90.0 | When Defaults Stop Being Real Strategy |
| 4 | 72.0 | 80.0 | 60.0 | 75.0 | 80.0 | 65.0 | 70.0 | When defaults become the real strategy |
| 5 | 80.0 | 85.0 | 75.0 | 80.0 | 85.0 | 70.0 | 75.0 | When defaults stop being an advantage |

- Baseline score: 75.0.
- Final score after five prompt-improvement iterations: 80.0.
- Final improvement over baseline: +5.0.
- Best score: 82.0, reached at iterations 2 and 3.
- The score did not improve monotonically. Iteration 4 dropped to 72.0 after the prompt addendum pushed harder on conversational flow, which appears to have hurt coherence and source-link clarity.
- The most consistent judge feedback was to improve synthesis: stop recapping sources one by one, make cross-source relationships explicit, keep source links close to the claims they support, and make the final implication feel earned.
- The best-performing addenda emphasized evidence-to-claim discipline, cross-source synthesis, source-link proximity, and concrete Abhijit-specific implications.
- A stronger next run should use a freshly refreshed or Supabase-backed latest run with populated insights, themes, clusters, and connections before treating this as a production prompt-quality result.
