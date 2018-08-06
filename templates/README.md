## 模板
* 参考
    * http://docs.jinkan.org/docs/jinja2/
    * https://www.jetbrains.com/help/pycharm/configuring-template-languages.html
    * https://github.com/flosch/pongo2/tree/master/template_tests
    * https://github.com/flosch/pongo2

* IDE支持(Pycharm). To configure a template language for a project
    1. Open the `Settings/Preferences` dialog box, and click the node `Python Template Languages`.
    2. In the Python Template Languages page, do the following:
        * From the `Template language` drop-down list, select the specific template language to be used in project.
            * 例如: 我们可以选择jinja2, 或者Django template, 它们和pongo2语法兼容
        * In the `Template file types area`, specify the types of files, where template tags will be recognized.
Note that in HTML, XHTML, and XML files templates are always recognized.
        * Use Add and Remove buttons to make up the desired list of file types.
        * Specify the directories where the templates in project will be stored. 
        * To do that, open the Project Structure page, select a directory to be declared as a template directory, and click /help/img/idea/2017.1/template_folder_icon.png.