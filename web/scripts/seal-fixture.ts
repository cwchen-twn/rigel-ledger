// Prints a blob sealed by src/lib/seal.ts, for internal/sealing's
// TestOpensABrowserBlob: bun scripts/seal-fixture.ts <runner public key, base64>
import { aad, seal } from '../src/lib/seal';

const pub = process.argv[2];
if (!pub) throw new Error('usage: bun scripts/seal-fixture.ts <public key base64>');
console.log(await seal(pub, '{"username":"alice","password":"otp"}', aad('credentials', 7, 'fake')));
