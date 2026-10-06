/*
 * What the runner keeps on its own disk (DATA_DIR, /data in the image): its
 * private key, and each connection's session state (cookies, a trusted
 * device id). Neither ever goes to the app. Both files are 0600.
 */
import { mkdir, readFile, rename, rm, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import type { State } from './connectors/types.ts';
import { newKey } from './sealing.ts';

export class Store {
  readonly dir: string;
  constructor(dir: string) {
    this.dir = dir;
  }

  /** The runner's private key, made on first start. */
  async key(): Promise<Buffer> {
    const path = join(this.dir, 'runner.key');
    try {
      const k = Buffer.from((await readFile(path, 'utf8')).trim(), 'base64');
      if (k.length !== 32) throw new Error(`${path} is not a runner key`);
      return k;
    } catch (err) {
      if ((err as NodeJS.ErrnoException).code !== 'ENOENT') throw err;
    }
    const { privateKey } = newKey();
    await this.write(path, privateKey.toString('base64') + '\n');
    return privateKey;
  }

  private statePath(name: string) {
    if (!/^[a-z0-9_-]+$/.test(name)) throw new Error(`bad state name ${name}`);
    return join(this.dir, 'state', `${name}.json`);
  }

  async state(name: string): Promise<State> {
    try {
      return JSON.parse(await readFile(this.statePath(name), 'utf8')) as State;
    } catch (err) {
      if ((err as NodeJS.ErrnoException).code === 'ENOENT') return {};
      throw err;
    }
  }

  async saveState(name: string, state: State) {
    await this.write(this.statePath(name), JSON.stringify(state));
  }

  async dropState(name: string) {
    await rm(this.statePath(name), { force: true });
  }

  private async write(path: string, content: string) {
    await mkdir(join(path, '..'), { recursive: true, mode: 0o700 });
    const tmp = `${path}.tmp`;
    await writeFile(tmp, content, { mode: 0o600 });
    await rename(tmp, path);
  }
}
