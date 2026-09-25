# Copyright 2026 Jan Wrobel <jan@mixedbit.org>
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http:#www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os
import tempfile

from base import TestBase, ENV_ID

BASE_CONFIG_MARKER = '# Drop sandbox base configuration file.'
ENV_CONFIG_MARKER = '# Drop sandbox environment-specific configuration file.'


class TestEdit(TestBase):

    def drop_edit(self, args='', editor=None, visual=None,
                  **subprocess_kwargs):
        env = os.environ.copy()
        env.pop('EDITOR', None)
        env.pop('VISUAL', None)
        if editor is not None:
            env['EDITOR'] = editor
        if visual is not None:
            env['VISUAL'] = visual
        return self.drop(f'edit {args}', env=env, **subprocess_kwargs)

    def test_prints_config_path_if_no_editor_set(self):
        self.drop_init()
        result = self.drop_edit(ENV_ID)
        self.assertSuccess(result)
        self.assertIn('Neither $VISUAL nor $EDITOR is set', result.stdout)
        self.assertIn(str(self.env_config_path()), result.stdout)

    def test_editor_opens_env_config(self):
        self.drop_init()
        result = self.drop_edit(ENV_ID, editor='cat')
        self.assertSuccess(result)
        self.assertIn(ENV_CONFIG_MARKER, result.stdout)

    def test_env_id_from_cwd(self):
        cwd = self.enterContext(
            tempfile.TemporaryDirectory(prefix='drop-e2e-tests'))
        self.drop('init', cwd=cwd)
        # 'echo' prints the path instead of opening it, so the test can
        # check which config drop selected.
        result = self.drop_edit(editor='echo', cwd=cwd)
        self.assertSuccess(result)
        env_id = str(cwd).replace('/', '-').strip('-')
        self.assertEqual(f'{self.env_config_path(env_id)}\n', result.stdout)

    def test_visual_takes_precedence_over_editor(self):
        self.drop_init()
        result = self.drop_edit(ENV_ID, editor='echo editor-was-used',
                                visual='cat')
        self.assertSuccess(result)
        self.assertIn(ENV_CONFIG_MARKER, result.stdout)
        self.assertNotIn('editor-was-used', result.stdout)

    def test_editor_arguments_precede_config_path(self):
        self.drop_init()
        # 'echo -n' omits the trailing newline only if -n is passed
        # before the config path, otherwise -n is printed literally.
        result = self.drop_edit(ENV_ID, editor='echo -n')
        self.assertSuccess(result)
        self.assertEqual(str(self.env_config_path()), result.stdout)

    def test_editor_failure_is_propagated(self):
        self.drop_init()
        result = self.drop_edit(ENV_ID, editor='false')
        self.assertEqual(1, result.returncode)

    def test_edit_base_opens_base_config(self):
        self.drop_init()
        result = self.drop_edit('base', editor='cat')
        self.assertSuccess(result)
        self.assertIn(BASE_CONFIG_MARKER, result.stdout)
        self.assertNotIn(ENV_CONFIG_MARKER, result.stdout)

    def test_invalid_env_id_rejected(self):
        self.drop_init()
        result = self.drop_edit('../invalid-id', editor='cat')
        self.assertEqual(1, result.returncode)
        self.assertIn('invalid environment ID', result.stderr)
        self.assertEqual('', result.stdout)

    def test_missing_env_config_rejected(self):
        self.drop_init()
        result = self.drop_edit('no-such-env', editor='cat')
        self.assertEqual(1, result.returncode)
        self.assertIn('environment "no-such-env" doesn\'t exist',
                      result.stderr)
        self.assertEqual('', result.stdout)

    def test_missing_base_config_rejected(self):
        # 'drop init' was not run, so the base config doesn't exist yet.
        result = self.drop_edit('base', editor='cat')
        self.assertEqual(1, result.returncode)
        self.assertIn("base.toml doesn't exist", result.stderr)
        self.assertEqual('', result.stdout)

    def test_edit_too_many_arguments(self):
        result = self.drop_edit('foo bar')
        self.assertEqual(1, result.returncode)
        self.assertEqual('Error: usage: drop edit [env-id or base]\n',
                         result.stderr)
