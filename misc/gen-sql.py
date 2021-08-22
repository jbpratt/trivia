#!/usr/bin/env python

import json

from typing import Any
from typing import Dict
from typing import List


def millionairedb_gen() -> None:
    path = "misc/millionairedb-questions.json"

    data: List[Dict[str, Any]]
    with open(path) as file:
        data = json.load(file)

    line = "INSERT OR IGNORE INTO\n\tquestions(question,answer,choices,categories,used,type,difficulty)\nVALUES\n"
    values: List[str] = []
    # misc/questions.json has been filtered to only contain these fields
    for row in data:
        question = row["question"]
        answer = row["answer"]
        choices = ",".join(row["choices"])
        categories = ",".join(list(dict.fromkeys(row["keywords"] + row["tags"])))

        values.append(
            f'\t(\n\t\t"{question}","{answer}","{choices}","{categories}",0,"",""\n\t)'
        )

    line += ",\n".join(values) + ";"

    with open("internal/trivia/millionairedb-questions.sql", "w") as file:
        file.write(line)


def opentdb_gen() -> None:
    path = "misc/opentdb-questions.json"

    data: Dict[str, Any]
    with open(path) as file:
        data = json.load(file)

    line = "INSERT OR IGNORE INTO\n\tquestions(question,answer,choices,categories,used,type,difficulty)\nVALUES\n"
    values: List[str] = []
    for row in data.values():
        category = row["category"]
        question_type = row["type"]
        difficulty = row["difficulty"]
        question = row["question"]
        correct_answer = row["correct_answer"]
        answers = row["incorrect_answers"]

        answers.append(correct_answer)
        choices = ",".join(answers)

        values.append(
            f'\t(\n\t\t"{question}","{correct_answer}","{choices}","{category}",0,"{question_type}","{difficulty}"\n\t)'
        )

    line += ",\n".join(values) + ";"

    with open("internal/trivia/opentdb-questions.sql", "w") as file:
        file.write(line)


def main() -> int:
    millionairedb_gen()
    opentdb_gen()
    return 0


if __name__ == "__main__":
    exit(main())
