# Newsletter prompt experiment observations

## Goal

Run a local experiment that creates a newsletter, scores it with a smaller LLM judge, feeds the judge feedback into a prompt improver, and repeats that loop for five iterations while tracking the score trajectory from the baseline.

## Initial repo observations

- The production newsletter synthesis path is in `backend/internal/knowledge/digest_synthesis.go`.
- The starting digest prompt version was `newsletter-stratechery-story-v5`.
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

## Prod-data rerun

- The eval wrapper loaded the production database URL from macOS Keychain for this historical rerun. Current runners use only `DATABASE_URL`; Supabase remains an Auth-only dependency.
- `npm run newsletter:eval -- -inspect-inputs` confirmed the Supabase-backed latest run before the rerun.
- Prod latest run timestamp: `2026-05-24T14:06:57Z`.
- Prod input counts: 299 X bookmarks, 5 YouTube items, 55 summaries, 166 insights, 8 themes, 1 insight cluster, 9 connections, 0 blockers.
- Prod report JSON: `data/runtime/newsletter-experiments/newsletter-eval-20260524T141241Z.json`.
- Prod report markdown: `data/runtime/newsletter-experiments/newsletter-eval-20260524T141241Z.md`.
- The prod experiment selected 5 insights for digest generation:
  - `youtube-YKZCU0ynEbs-insight-1`
  - `x-2056428390773825701-insight-1`
  - `x-2058156429559636069-insight-2`
  - `x-2056475000027410673-insight-3`
  - `x-2056751628867285435-insight-3`

| Iteration | Overall | Grounding | Synthesis | Voice | Usefulness | Structure | Links | Subject |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 0 | 86.0 | 90.0 | 85.0 | 87.0 | 88.0 | 82.0 | 80.0 | When Building Gets Easy, Judgment Gets Scarce |
| 1 | 82.0 | 85.0 | 80.0 | 83.0 | 84.0 | 78.0 | 80.0 | When Convenience Pretends to Be Evidence |
| 2 | 88.0 | 85.0 | 90.0 | 87.0 | 90.0 | 89.0 | 85.0 | Validate the human layer before scaling |
| 3 | 82.0 | 85.0 | 80.0 | 78.0 | 85.0 | 75.0 | 90.0 | Shortcuts are useful until they hide the test |
| 4 | 88.0 | 85.0 | 90.0 | 87.0 | 90.0 | 85.0 | 90.0 | The new bottleneck is judgment, not speed |
| 5 | 83.0 | 85.0 | 80.0 | 84.0 | 90.0 | 80.0 | 85.0 | The hidden bottleneck before scaling |

- Prod baseline score: 86.0.
- Prod final score after five prompt-improvement iterations: 83.0.
- Prod final delta from baseline: -3.0.
- Prod best score: 88.0, reached at iterations 2 and 4.
- The best prod addendum was iteration 4's input addendum, not the final generated addendum. It emphasized making numerical claims precise, keeping the thesis visible, using phone-first structure, tying judgments to linked evidence, and ending with a practical action.
- Main product lesson: this should not blindly accept the last prompt revision. The loop needs checkpointing and champion selection: keep the highest-scoring prompt addendum, and only promote a new addendum when it beats the current baseline or champion by a margin.
- The judge repeatedly pushed on source-link proximity, explicit synthesis, phone readability, and avoiding unsupported broad claims. With graph context present, the baseline was already strong, so later prompt changes were more likely to overfit or degrade structure.

## Prompt promotion

- The May 24 production newsletter prompt was promoted to `newsletter-stratechery-story-v6`.
- The promoted instructions come from the best prod champion pattern, not from the final iteration.
- The incorporated prompt learning asks the digest generator to keep numerical claims precise, keep the central thesis visible across paragraphs, use graph themes/clusters/connections as connective tissue, preserve phone-first paragraph flow, tie judgments to linked evidence, and end with one practical Abhijit-specific next move.
- Backend prompt bodies were moved into `backend/prompts` so production prompts, experiment prompts, Ask Second Brain prompts, source synthesis prompts, and digest illustration prompts can be reviewed in one place.

## June 6 Langfuse rerun

- The production database was unreachable from the local machine at `10.104.0.3:5432`, so the controlled rerun used `data/runtime/latest-knowledge-run.json`: 14 summaries and no populated insight/graph collections.
- The observability fix produced stable Langfuse traces with one root name, a session ID, release and version, fixed trace tags, named child observations, token/cost data, and programmatic scores.
- The first calibrated v7 run scored 72.5 at baseline and 78.3 after one revision, a +5.8 point champion improvement.
- The champion feedback asked for a source-backed opening tension, links adjacent to factual and interpretive claims, explicit cross-source connections, and a final Abhijit-specific decision filter.
- Those instructions were promoted into `newsletter-stratechery-story-v8`.
- A baseline-only v8 verification scored 77.5, five points above the calibrated v7 baseline and within 0.8 points of the v7 optimized champion.
- The managed prompt `second-brain/digest-newsletter` is Langfuse version 3 with `production` and `latest` labels.
- The final verification trace is `3360fa2218e5ed16e5e62679f6a5e39f`: 3 observations, 11 scores, $0.0087786 total model cost, and 21.934 seconds latency.
- The eval harness now rejects internally inconsistent judge scales, retries transient provider failures once, preserves partial-run scores, and selects the highest-scoring prompt addendum instead of blindly treating the final iteration as the champion.
