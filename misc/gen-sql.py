#!/usr/bin/env python

import json

from typing import Any
from typing import Dict
from typing import List
from typing import TextIO

INSERT_LINE = "INSERT OR IGNORE INTO\n\tquestions(question,answer,choices,categories,used,source,type,difficulty)\nVALUES\n"


def jackbox_3_murder_gen(output: TextIO) -> None:
    path = "misc/jackbox-3-murder-trivia.json"
    data: List[Dict[str, Union[str, List[str]]]]
    with open(path) as file:
        data = json.load(file)

    values: List[str] = []
    for row in data:
        question = row["question"].replace("\"", "\'")
        answer = row["answer"].replace("\"", "\'")
        choices = ",".join(row["options"]).replace("\"", "\'")

        values.append(
            f'\t("{question}","{answer}","{choices}","",0,"jackbox_3_murder","","")'
        )

    output.write(",\n".join(values))


def millionairedb_gen(output: TextIO) -> None:
    path = "misc/millionairedb-questions.json"

    data: List[Dict[str, Any]]
    with open(path) as file:
        data = json.load(file)

    values: List[str] = []
    # misc/questions.json has been filtered to only contain these fields
    for row in data:
        question = row["question"]
        answer = row["answer"]
        choices = ",".join(row["choices"])
        categories = ",".join(list(dict.fromkeys(row["keywords"] + row["tags"])))

        values.append(
            f'\t("{question}","{answer}","{choices}","{categories}",0,"millionairedb","","")'
        )

    output.write(",\n".join(values))


def opentdb_gen(output: TextIO) -> None:
    path = "misc/opentdb-questions.json"

    data: Dict[str, Any]
    with open(path) as file:
        data = json.load(file)

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
            f'\t("{question}","{correct_answer}","{choices}","{category}",0,"opentdb","{question_type}","{difficulty}")'
        )

    output.write(",\n".join(values))


def main() -> int:

    with open("internal/trivia/questions.sql", "w") as file:
        file.write(INSERT_LINE)
        millionairedb_gen(file)
        file.write(",\n")
        opentdb_gen(file)
        file.write(",\n")
        jackbox_3_murder_gen(file)
        file.write(";")

    return 0


if __name__ == "__main__":
    exit(main())
