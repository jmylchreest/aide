"""Security test file for Python security analyzer."""

import os
import pickle
import subprocess
import hashlib
import yaml


def sql_injection(cursor, user_id):
    """SQL injection via f-string."""
    cursor.execute(f"SELECT * FROM users WHERE id = {user_id}")


def eval_usage(user_input):
    """Use of eval with user input."""
    result = eval(user_input)
    return result


def command_injection(user_input):
    """Command injection via shell=True."""
    subprocess.call("echo " + user_input, shell=True)


def pickle_deserialize(data):
    """Unsafe deserialization."""
    return pickle.loads(data)


def yaml_unsafe_load(content):
    """Unsafe YAML loading."""
    return yaml.load(content)


def weak_hash(data):
    """Use of MD5."""
    return hashlib.md5(data).hexdigest()


def safe_query(cursor, user_id):
    """Parameterized query — should NOT trigger."""
    cursor.execute("SELECT * FROM users WHERE id = %s", (user_id,))


# This is a comment with eval() in it — should NOT trigger
# pickle.loads is also mentioned in a comment
