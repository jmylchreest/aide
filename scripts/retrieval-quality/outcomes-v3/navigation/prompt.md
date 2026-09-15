Trace OpenCode model-usage accounting through this source snapshot. Do not edit files. Give a concise answer with file:line evidence for each item:

1. Which event and part type trigger model-usage collection, and how does that event reach the writer? Does message.updated itself contribute usage?
2. For a step-finish part with id="step-7", messageID="message-2", sessionID="session-1", and tokens={input:11,output:13,reasoning:5,cache:{read:17,write:3}}, give the persisted observation identity, normalized counters, time basis, and counters or model/provider metadata that remain unknown.
3. The same part is broadcast three times: its first write fails before persisting anything, its second succeeds, and its third repeats unchanged. Then a fourth broadcast has the same IDs but input:12 and otherwise unchanged counters, and that write succeeds. How many writer attempts and distinct stored usage variants result? Explain the in-memory cache and durable deduplication roles.
4. What does a project-wide usage report include for that identity after the changed variant arrives? Can a date filter selecting only the earlier variant restore its counters?
5. Do these session usage observations add to tool-event counts or byte-based token estimates? What can the counters establish about total session completeness, billed cost, savings, or output quality? State explicitly whether cache counters may be added again to normalized input_tokens and whether reported_output_tokens may be summed with reasoning_output_tokens when their overlap is unknown.

Use the supplied source as authority. Do not speculate beyond its contracts.
