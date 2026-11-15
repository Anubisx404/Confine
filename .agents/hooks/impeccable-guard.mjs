import fs from "node:fs";
import path from "node:path";

function readStdin() {
  return fs.readFileSync(0, "utf8");
}

function isWebSource(filePath) {
  if (!filePath) return false;
  const normalized = String(filePath).replace(/\\/g, "/");
  return /(?:^|\/)web\/[^/]+\.(html|css|js)$/i.test(normalized);
}

function checkImpeccableRules(text, filePath) {
  if (!text) return [];
  const violations = [];
  const lowerPath = String(filePath || "").toLowerCase();

  if (/border-(?:left|right)\s*:\s*(?:[2-9]|\d{2,})(?:\.\d+)?px\b/i.test(text)) {
    violations.push("Impeccable Ban 1: border-left/border-right thicker than 1px is forbidden");
  }
  if (/(?:-webkit-)?background-clip\s*:\s*text/i.test(text)) {
    violations.push("Impeccable Ban 2: gradient text (background-clip: text) is forbidden");
  }
  if (/backdrop-filter\s*:/i.test(text)) {
    violations.push("Impeccable: backdrop-filter glassmorphism is forbidden");
  }
  if (/(?:linear|radial|conic)-gradient\s*\(/i.test(text)) {
    violations.push("Impeccable: gradient backgrounds/accents are forbidden");
  }
  if (/\b(?:Inter|Roboto|Outfit|Space Grotesk|Plus Jakarta|Fraunces|Newsreader|Playfair Display|DM Sans|Instrument Serif)\b/.test(text)) {
    violations.push("Impeccable: reflex banned typeface detected");
  }
  if (/\bctx\.filter\s*=|\.style\.filter\s*=/.test(text)) {
    violations.push("Performance guard: ctx.filter and element.style.filter are forbidden on live canvases");
  }
  if (/(?:\/\*|\*\/|<!--)/.test(text) || (!lowerPath.endsWith(".html") && /(?:^|\n)\s*\/\//.test(text))) {
    violations.push("Code hygiene: source comments are forbidden in Confine web sources");
  }
  return violations;
}

function handleEvent(payload) {
  const eventName = payload && payload.hookEventName;

  if (eventName === "SessionStart") {
    return {
      injectSteps: [
        {
          systemMessage: {
            systemMessage:
              "Confine Impeccable Guard Active: follow .impeccable.md (fast, dense, unimpressed; OKLCH graphite + warm ochre; zero gradients, zero backdrop-filter, zero side-stripe >1px borders, zero reflex fonts, zero canvas/CSS filter blur, zero comments)."
          }
        }
      ]
    };
  }

  if (eventName === "PreToolUse") {
    const toolCall = payload.toolCall || {};
    const toolName = String(toolCall.name || "").toLowerCase();
    const args = toolCall.args || {};
    if (toolName === "write_file" || toolName === "edit_file") {
      const targetPath = args.Path || args.path || "";
      if (isWebSource(targetPath)) {
        const snippet = args.Content || args.Replacement || "";
        const issues = checkImpeccableRules(snippet, targetPath);
        if (issues.length > 0) {
          return {
            decision: "deny",
            reason: issues.join("; ")
          };
        }
      }
    }
    return { decision: "allow" };
  }

  if (eventName === "PostToolUse") {
    return {};
  }

  return {};
}

try {
  const raw = readStdin().replace(/^\uFEFF/, "");
  const payload = raw.trim() ? JSON.parse(raw) : {};
  const result = handleEvent(payload);
  process.stdout.write(JSON.stringify(result));
} catch (err) {
  process.stdout.write(JSON.stringify({ decision: "allow", note: String(err && err.message ? err.message : err) }));
}
