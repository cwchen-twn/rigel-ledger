import type { Connector } from './types.ts';
import { SyncError } from './types.ts';

/*
 * The pretend institution of integrations/fake-runner, for development and
 * the end-to-end check: any username; password "wrong" fails with
 * bad_credentials; password "otp" asks for a one-time code, which is 123456.
 * One checking account, three transactions and a balance, the same ids every
 * day. Offered only with TW_SYNC_FAKE=1.
 */
export const fake: Connector = {
  id: 'fake',
  name: 'Fake Bank',
  country: 'ZZ',
  fields: [
    { name: 'username', label: 'Username', kind: 'text' },
    { name: 'password', label: 'Password', kind: 'secret' },
  ],
  async sync(ctx) {
    const { username, password } = ctx.credentials;
    if (!username) throw new SyncError('bad_credentials');
    if (password === 'wrong') throw new SyncError('bad_credentials');
    if (password === 'otp') {
      // A trusted device skips the code next time, as real banks do.
      if (ctx.state.trusted !== true) {
        const code = await ctx.ask({ kind: 'otp', prompt: 'Fake Bank sent a code by SMS (it is 123456)', ttlSeconds: 300 });
        if (code.trim() !== '123456') throw new SyncError('bad_otp');
        await ctx.saveState({ trusted: true });
      }
    }
    const day = new Date().toISOString().slice(0, 10);
    const acct = 'fake-chk-001';
    return {
      label: `Fake Bank ${day}`,
      accounts: [{ id: acct, label: 'Fake Bank checking ...001', currency: 'TWD' }],
      rows: [
        { kind: 'transaction', account: acct, id: `${day}-1`, date: day, amount: '-120', description: '7-ELEVEN 0931' },
        { kind: 'transaction', account: acct, id: `${day}-2`, date: day, amount: '-1580', description: 'PX MART' },
        { kind: 'transaction', account: acct, id: `${day}-3`, date: day, amount: '42000', description: 'SALARY ACME' },
        { kind: 'balance', account: acct, id: `${day}-bal`, date: day, amount: '40300' },
      ],
    };
  },
};
