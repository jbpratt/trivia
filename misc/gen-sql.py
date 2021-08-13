#!/usr/bin/env python

import json

from typing import Any
from typing import Dict
from typing import List


def main() -> int:
    path = "misc/questions.json"

    data: List[Dict[str, Any]]
    with open(path) as file:
        data = json.load(file)

    line = "INSERT OR IGNORE INTO\n\tquestions(question,answer,choices,categories,used)\nVALUES\n"
    values: List[str] = []
    # misc/questions.json has been filtered to only contain these fields
    for row in data:
        question = row["question"]
        answer = row["answer"]
        choices = ",".join(row["choices"])
        categories = ",".join(list(dict.fromkeys(row["keywords"] + row["tags"])))

        values.append(f'\t(\n\t\t"{question}","{answer}","{choices}","{categories}",0\n\t)')

    line += ",\n".join(values) + ";"

    with open("internal/trivia/questions.sql", "w") as file:
        file.write(line)

    return 0


if __name__ == "__main__":
    exit(main())
