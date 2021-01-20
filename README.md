# goreload
## use fsnotify to watch dir recursively,and auto rebuild and run when find file change

## Usage of goreload:  
  -d    run Program in debug mode  
  -e string  
        exclude monitor dir,can list multiple, -e "dir1 dir2 dir3" (default ".git")  
  -r string  
        run Program after auto build  
   
 
 ### cd workspace,run goreload,when file modified,this will auto build code and run  
 for example:  
 goreload -r "./simple_web -p 8081"
