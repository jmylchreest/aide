import {test,expect} from "bun:test";
import {replacementTarget} from "./src/core/context-pruning/replacement";
test("OpenCode aide text replacement preserves metadata",()=>{const payload={content:[{type:"text",text:"original",annotations:{priority:1}}],_meta:{work:"receipt"}};const target=replacementTarget("aide_code_outline",payload);expect(target?.text).toBe("original");expect(target?.replace("short")).toEqual({...payload,content:[{...payload.content[0],text:"short"}]});expect(payload.content[0].text).toBe("original");});
